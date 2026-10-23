//go:build windows

package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Prefixes of CRYPT_PROVIDER_SGNR and CRYPT_PROVIDER_CERT, owned by WinVerifyTrust.
type providerSigner struct {
	Size             uint32
	VerifyAsOf       windows.Filetime
	CertificateCount uint32
	Certificates     *providerCertificate
}
type providerCertificate struct {
	Size        uint32
	Certificate *windows.CertContext
}

func verifiedPrerequisite(p Prerequisite, path string) (_ *os.File, err error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("invalid component path")
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("could not open the component")
	}
	file := os.NewFile(uintptr(handle), path)
	defer func() {
		if err != nil {
			file.Close()
		}
	}()
	hash := sha256.New()
	count, err := io.Copy(hash, io.LimitReader(file, (512<<20)+1))
	if err != nil || count > 512<<20 || !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), p.SHA256) {
		return nil, fmt.Errorf("component failed integrity verification")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("could not verify the component")
	}
	info := windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: name, File: handle}
	data := windows.WinTrustData{Size: uint32(unsafe.Sizeof(windows.WinTrustData{})), UIChoice: windows.WTD_UI_NONE, RevocationChecks: windows.WTD_REVOKE_NONE, UnionChoice: windows.WTD_CHOICE_FILE, StateAction: windows.WTD_STATEACTION_VERIFY, ProvFlags: windows.WTD_CACHE_ONLY_URL_RETRIEVAL, FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(&info)}
	defer func() {
		data.StateAction = windows.WTD_STATEACTION_CLOSE
		_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data)
	}()
	if windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, &data) != nil {
		return nil, fmt.Errorf("component digital signature is invalid")
	}
	trust := windows.NewLazySystemDLL("wintrust.dll")
	provider, _, _ := trust.NewProc("WTHelperProvDataFromStateData").Call(uintptr(data.StateData))
	if provider == 0 {
		return nil, fmt.Errorf("could not verify the component publisher")
	}
	signerPtr, _, _ := trust.NewProc("WTHelperGetProvSignerFromChain").Call(provider, 0, 0, 0)
	if signerPtr == 0 {
		return nil, fmt.Errorf("could not verify the component publisher")
	}
	var signer providerSigner
	if windows.ReadProcessMemory(windows.CurrentProcess(), signerPtr, (*byte)(unsafe.Pointer(&signer)), unsafe.Sizeof(signer), nil) != nil {
		return nil, fmt.Errorf("could not read the component publisher")
	}
	if signer.CertificateCount == 0 || signer.Certificates == nil || signer.Certificates.Certificate == nil {
		return nil, fmt.Errorf("component has no valid publisher")
	}
	oid := []byte("2.5.4.3\x00")
	size := windows.CertGetNameString(signer.Certificates.Certificate, windows.CERT_NAME_ATTR_TYPE, 0, unsafe.Pointer(&oid[0]), nil, 0)
	if size < 2 || size > 4096 {
		return nil, fmt.Errorf("component has no valid publisher")
	}
	cn := make([]uint16, size)
	windows.CertGetNameString(signer.Certificates.Certificate, windows.CERT_NAME_ATTR_TYPE, 0, unsafe.Pointer(&oid[0]), &cn[0], size)
	expected := strings.TrimSuffix(strings.TrimPrefix(p.Publisher, "CN="), ",")
	if windows.UTF16ToString(cn) != expected {
		return nil, fmt.Errorf("component publisher does not match the expected publisher")
	}
	return file, nil
}
