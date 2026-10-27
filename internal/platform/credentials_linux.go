package platform

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
	"golang.org/x/sys/unix"
)

const keyReferenceFile = "key.secret-service"
const keyService = "org.sdrive.credentials"

var keyGet = keyring.Get
var keySet = keyring.Set

func CheckKey(dir string) error { _, err := os.Stat(filepath.Join(dir, keyReferenceFile)); return err }
func Key(dir string) (string, error) {
	fd, err := unix.Open(filepath.Join(dir, "key.lock"), unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return "", err
	}
	defer unix.Close(fd)
	if err = unix.Flock(fd, unix.LOCK_EX); err != nil {
		return "", err
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	file, err := os.OpenFile(filepath.Join(dir, keyReferenceFile), os.O_RDONLY|unix.O_NOFOLLOW, 0)
	var ref []byte
	if err == nil {
		ref, err = io.ReadAll(io.LimitReader(file, 65))
		file.Close()
	}
	if err == nil {
		if decoded, e := hex.DecodeString(string(ref)); e != nil || len(decoded) != 32 {
			return "", fmt.Errorf("Invalid credential reference. Preserve your profile and cache.")
		}
		value, e := keyGet(keyService, string(ref))
		if e != nil {
			return "", fmt.Errorf("Could not unlock the desktop credential store. Unlock your keyring and retry; preserve your profile and cache.")
		}
		if b, e := base64.RawStdEncoding.DecodeString(value); e != nil || len(b) != 32 {
			return "", fmt.Errorf("Invalid credential key. Preserve your profile and cache.")
		}
		return value, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	for _, name := range []string{"rclone.conf", "proxy.enc", AccessFile, "key.dpapi", "application-lock.dpapi"} {
		info, e := os.Lstat(filepath.Join(dir, name))
		if e == nil && (info.Size() > 0 || !info.Mode().IsRegular()) {
			return "", fmt.Errorf("Credential key missing. Preserve the profile and cache; use portable export to move between operating systems.")
		}
		if e != nil && !os.IsNotExist(e) {
			return "", e
		}
	}
	raw, id := make([]byte, 32), make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", err
	}
	if _, err = rand.Read(id); err != nil {
		return "", err
	}
	reference, value := hex.EncodeToString(id), base64.RawStdEncoding.EncodeToString(raw)
	if err = keySet(keyService, reference, value); err != nil {
		return "", fmt.Errorf("Could not unlock the desktop credential store. Unlock your keyring and retry; preserve your profile and cache.")
	}
	// The marker contains only a random lookup ID; the secret never reaches disk.
	if err = AtomicWrite(filepath.Join(dir, keyReferenceFile), []byte(reference)); err != nil {
		return "", err
	}
	return value, nil
}
func protectedCipher(dir string) (cipher.AEAD, error) {
	value, err := Key(dir)
	if err != nil {
		return nil, err
	}
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func ProtectData(dir string, data []byte) ([]byte, error) {
	gcm, err := protectedCipher(dir)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, data, []byte("sdrive-access-v1")), nil
}
func UnprotectData(dir string, data []byte) ([]byte, error) {
	gcm, err := protectedCipher(dir)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("invalid protected record")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte("sdrive-access-v1"))
}

// Temporary probes own their keyring entries and may remove only their own references.
func CleanupProbeKey(dir string) error {
	ref, err := os.ReadFile(filepath.Join(dir, keyReferenceFile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(ref) != 64 || strings.ContainsAny(string(ref), "/\\") {
		return fmt.Errorf("invalid credential reference")
	}
	return keyring.Delete(keyService, string(ref))
}
