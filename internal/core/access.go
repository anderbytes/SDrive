package core

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/argon2"
	"sdrive/internal/platform"
)

const accessFile = "application-lock.dpapi"

type AccessPolicy struct {
	IdleMinutes    int  `json:"idleMinutes"`
	DisconnectIdle bool `json:"disconnectIdle"`
	Reconnect      bool `json:"reconnect"`
}

type AccessStatus struct {
	Theme        string       `json:"theme"`
	SetupPending bool         `json:"setupPending"`
	Language     string       `json:"language"`
	Enabled      bool         `json:"enabled"`
	Locked       bool         `json:"locked"`
	Policy       AccessPolicy `json:"policy"`
	RetryAt      time.Time    `json:"retryAt"`
	Error        string       `json:"error"`
	Warning      string       `json:"warning"`
}

type accessRecord struct {
	Version  int          `json:"version"`
	Salt     []byte       `json:"salt"`
	PIN      []byte       `json:"pin"`
	Recovery []byte       `json:"recovery"`
	Policy   AccessPolicy `json:"policy"`
	Failures int          `json:"failures"`
	RetryAt  time.Time    `json:"retryAt"`
}

type accessProtection struct {
	record         *accessRecord
	locked         bool
	error          string
	warning        string
	epoch          uint64
	changed        func(bool)
	now            func() time.Time
	idle           func() (time.Duration, error)
	idleRunning    bool
	idleDone       bool
	reconnect      []string
	startupPending bool
}

func (s *Service) accessTime() time.Time {
	if s.access.now != nil {
		return s.access.now()
	}
	return time.Now()
}

func validAccessPolicy(p AccessPolicy) bool {
	return p.IdleMinutes >= 0 && p.IdleMinutes <= 1440 && (!p.DisconnectIdle || p.IdleMinutes > 0) && (!p.Reconnect || p.DisconnectIdle)
}

func validPIN(pin string) bool {
	if len(pin) < 6 || len(pin) > 12 {
		return false
	}
	for _, c := range pin {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func (s *Service) loadAccess() {
	s.access.idle = platform.InputIdle
	file, err := os.Open(filepath.Join(s.dir, accessFile))
	var b []byte
	if err == nil {
		defer file.Close()
		b, err = io.ReadAll(io.LimitReader(file, 16385))
	}
	if os.IsNotExist(err) && !s.store.AccessProtected {
		return
	}
	s.access.locked = true
	s.access.startupPending = true
	if err == nil && len(b) <= 16384 {
		b, err = platform.UnprotectData(b)
		var record accessRecord
		if err == nil && json.Unmarshal(b, &record) == nil && record.Version == 1 && len(record.Salt) == 16 && len(record.PIN) == 32 && len(record.Recovery) == 32 && record.Failures >= 0 && record.Failures <= 64 && validAccessPolicy(record.Policy) {
			s.access.record = &record
			return
		}
	}
	s.access.error = "Application lock could not be read. Preserve your profile and follow the lock recovery instructions."
}

func (s *Service) saveAccess(record accessRecord) error {
	b, err := json.Marshal(record)
	if err == nil {
		b, err = platform.ProtectData(b)
	}
	if err == nil {
		err = atomicWrite(filepath.Join(s.dir, accessFile), b)
	}
	if err != nil {
		return fmt.Errorf("Could not save application lock. Your connections and cache are preserved.")
	}
	s.access.record = &record
	return nil
}

// Caller holds s.mu. This gate also protects native tray calls, not just the bridge.
func (s *Service) requireUnlocked() error {
	if s.access.locked {
		return fmt.Errorf("Unlock SDrive before managing connections.")
	}
	return nil
}

func (s *Service) accessAllowed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.requireUnlocked()
}

func (s *Service) AccessStatus() AccessStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accessStatus()
}

func (s *Service) accessStatus() AccessStatus {
	status := AccessStatus{Theme: s.store.Settings.Theme, SetupPending: !s.access.locked && s.session != nil, Language: s.store.Settings.Language, Enabled: s.access.record != nil || s.store.AccessProtected || s.access.error != "", Locked: s.access.locked, Error: s.access.error, Warning: s.access.warning, Policy: AccessPolicy{IdleMinutes: 15}}
	if r := s.access.record; r != nil {
		status.Policy, status.RetryAt = r.Policy, r.RetryAt
	}
	return status
}

func (s *Service) lockedView() View {
	return View{Access: s.accessStatus(), Settings: Settings{Language: s.store.Settings.Language, Theme: s.store.Settings.Theme}, Providers: []Provider{}, Connections: []ConnectionView{}, ImportDrafts: []ImportDraft{}, Letters: []string{}}
}

func (s *Service) lockManagement() {
	s.access.locked = true
	if ss := s.session; ss != nil {
		go func(id string) {
			if err := s.endConfig(id); err != nil {
				s.mu.Lock()
				s.access.warning = "Setup cancellation could not be confirmed. Retry after unlocking; configuration and cache are preserved."
				s.accessChanged()
				s.mu.Unlock()
			}
		}(ss.ID)
	}
	s.accessChanged()
}

// Notifications contain no connection data and never wait for the desktop thread.
func (s *Service) accessChanged() {
	s.access.epoch++
	s.snapshotID++
	if s.access.changed != nil {
		s.access.changed(s.access.locked)
	}
}

func SetAccessChanged(s *Service, changed func(bool)) { s.access.changed = changed }

func (s *Service) verifyAccess(secret string, recovery bool) error {
	r := s.access.record
	if r == nil || s.access.error != "" {
		return fmt.Errorf("Application lock is unavailable. Follow the lock recovery instructions.")
	}
	if s.accessTime().Before(r.RetryAt) {
		return fmt.Errorf("Too many attempts. Wait before trying again.")
	}
	var candidate []byte
	expected := r.PIN
	if recovery {
		if len(secret) > 128 {
			secret = ""
		}
		digest := sha256.Sum256([]byte(secret))
		candidate, expected = digest[:], r.Recovery
	} else {
		if !validPIN(secret) {
			secret = ""
		}
		candidate = argon2.IDKey([]byte(secret), r.Salt, 3, 64*1024, 1, 32)
	}
	next := *r
	if subtle.ConstantTimeCompare(candidate, expected) != 1 {
		next.Failures = min(next.Failures+1, 64)
		if next.Failures >= 5 {
			next.RetryAt = s.accessTime().Add(min(30*time.Second*time.Duration(1<<min(next.Failures-5, 5)), 15*time.Minute))
		}
		if err := s.saveAccess(next); err != nil {
			return err
		}
		return fmt.Errorf("The PIN or recovery code is incorrect.")
	}
	next.Failures, next.RetryAt = 0, time.Time{}
	return s.saveAccess(next)
}

func newAccessRecord(pin string, policy AccessPolicy) (accessRecord, string, error) {
	if !validPIN(pin) {
		return accessRecord{}, "", fmt.Errorf("Enter a PIN containing 6 to 12 digits.")
	}
	if !validAccessPolicy(policy) {
		return accessRecord{}, "", fmt.Errorf("Choose a valid inactivity policy between 0 and 1440 minutes.")
	}
	r := accessRecord{Version: 1, Salt: make([]byte, 16), Policy: policy}
	code := make([]byte, 20)
	if _, err := rand.Read(r.Salt); err != nil {
		return r, "", err
	}
	if _, err := rand.Read(code); err != nil {
		return r, "", err
	}
	recovery := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(code)
	digest := sha256.Sum256([]byte(recovery))
	r.Recovery = digest[:]
	r.PIN = argon2.IDKey([]byte(pin), r.Salt, 3, 64*1024, 1, 32)
	return r, recovery, nil
}

func (s *Service) ConfigureAccess(currentPIN, newPIN string, policy AccessPolicy) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.access.idleRunning {
		err := fmt.Errorf("Wait for the inactivity disconnection to finish.")
		return "", err
	}
	if err := s.requireUnlocked(); err != nil {
		return "", err
	}
	if err := s.persistenceReady(); err != nil {
		return "", err
	}
	if !validAccessPolicy(policy) {
		return "", fmt.Errorf("Choose a valid inactivity policy between 0 and 1440 minutes.")
	}
	if s.access.record != nil {
		if err := s.verifyAccess(currentPIN, false); err != nil {
			return "", err
		}
		if newPIN == "" {
			next := *s.access.record
			next.Policy = policy
			if err := s.saveAccess(next); err != nil {
				return "", err
			}
			s.accessChanged()
			return "", nil
		}
	}
	r, code, err := newAccessRecord(newPIN, policy)
	if err != nil {
		return "", err
	}
	// Commit the downgrade/missing-record guard before claiming protection is active.
	previous := s.store.AccessProtected
	s.store.AccessProtected = true
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.AccessProtected = previous
		return "", err
	}
	if err := s.saveAccess(r); err != nil {
		if !previous {
			s.store.AccessProtected = false
			if rollback := saveStore(s.dir, s.store); rollback != nil {
				s.store.AccessProtected = true
				s.access.locked = true
				s.access.error = "Application lock could not be read. Preserve your profile and follow the lock recovery instructions."
				s.accessChanged()
			}
		}
		return "", err
	}
	s.accessChanged()
	return code, nil
}

func (s *Service) DisableAccess(pin string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.access.idleRunning {
		err := fmt.Errorf("Wait for the inactivity disconnection to finish.")
		return err
	}
	if err := s.requireUnlocked(); err != nil {
		return err
	}
	if err := s.verifyAccess(pin, false); err != nil {
		return err
	}
	s.store.AccessProtected = false
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.AccessProtected = true
		return err
	}
	if err := os.Remove(filepath.Join(s.dir, accessFile)); err != nil {
		s.store.AccessProtected = true
		if rollback := saveStore(s.dir, s.store); rollback != nil {
			s.access.locked = true
			s.access.error = "Could not save application lock. Your connections and cache are preserved."
			s.accessChanged()
		}
		return fmt.Errorf("Could not remove application lock; protection remains enabled.")
	}
	s.access.record = nil
	s.access.reconnect = nil
	s.accessChanged()
	return nil
}

func (s *Service) Lock() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.access.locked {
		return nil
	}
	if s.access.record == nil {
		return fmt.Errorf("Set an application PIN before locking SDrive.")
	}
	s.lockManagement()
	return nil
}

func (s *Service) UnlockAccess(pin string) error {
	s.mu.Lock()
	if err := s.verifyAccess(pin, false); err != nil {
		s.mu.Unlock()
		return err
	}
	s.access.locked = false
	s.accessChanged()
	s.mu.Unlock()
	go s.resumeAccess()
	return nil
}

func (s *Service) RecoverAccess(code, pin string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validPIN(pin) {
		return "", fmt.Errorf("Enter a PIN containing 6 to 12 digits.")
	}
	if err := s.verifyAccess(code, true); err != nil {
		return "", err
	}
	r, replacement, err := newAccessRecord(pin, s.access.record.Policy)
	if err == nil {
		err = s.saveAccess(r)
	}
	// Stay locked until the replacement code has been shown and the new PIN entered.
	return replacement, err
}

func (s *Service) resumeAccess() {
	s.mu.Lock()
	if s.access.locked || s.access.idleRunning {
		s.mu.Unlock()
		return
	}
	ids := s.access.reconnect
	s.access.reconnect = nil
	startup := s.access.startupPending
	s.access.startupPending = false
	s.mu.Unlock()
	if startup {
		s.mu.Lock()
		needsEngine := s.rpc == nil
		s.mu.Unlock()
		if needsEngine {
			_ = s.RestartEngine()
		}
		AutoMount(s)
	}
	for _, id := range ids {
		_ = s.Mount(id)
	}
}

// CheckInactivity is a package function: the bridge cannot forge idle measurements.
func CheckInactivity(s *Service) {
	s.mu.Lock()
	r := s.access.record
	if r == nil || r.Policy.IdleMinutes == 0 || s.access.idle == nil {
		s.mu.Unlock()
		return
	}
	idle, err := s.access.idle()
	if err != nil {
		s.mu.Unlock()
		return
	}
	if idle < time.Duration(r.Policy.IdleMinutes)*time.Minute {
		s.access.idleDone = false
		s.mu.Unlock()
		return
	}
	if s.access.idleDone || s.access.idleRunning {
		s.mu.Unlock()
		return
	}
	s.access.idleDone = true
	s.lockManagement()
	if !r.Policy.DisconnectIdle {
		s.mu.Unlock()
		return
	}
	s.access.idleRunning = true
	ids := []string{}
	for id, mounted := range s.mounted {
		if mounted {
			ids = append(ids, id)
		}
	}
	s.mu.Unlock()
	for _, id := range ids {
		err := s.unmount(id)
		s.mu.Lock()
		if err != nil {
			s.access.warning = "Some drives could not disconnect safely after inactivity. Check uploads and open files."
		} else if r.Policy.Reconnect {
			s.access.reconnect = append(s.access.reconnect, id)
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	s.access.idleRunning = false
	unlocked := !s.access.locked
	s.mu.Unlock()
	if unlocked {
		s.resumeAccess()
	}
}
