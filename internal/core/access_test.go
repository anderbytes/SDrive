package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func enableTestAccess(t *testing.T, s *Service, policy AccessPolicy) string {
	t.Helper()
	code, err := s.ConfigureAccess("", "123456", policy)
	if err != nil || len(code) != 32 {
		t.Fatalf("configure access did not produce a recovery code: %v", err)
	}
	return code
}

func TestApplicationLockRecoveryAndRestart(t *testing.T) {
	s := testService(t, nil)
	code := enableTestAccess(t, s, AccessPolicy{IdleMinutes: 15})
	if err := s.Lock(); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(s.dir, nil, "")
	if err != nil || !reopened.AccessStatus().Locked {
		t.Fatalf("restart did not lock: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(s.dir, accessFile))
	if err != nil || bytes.Contains(raw, []byte("123456")) || bytes.Contains(raw, []byte(code)) {
		t.Fatal("unprotected access record")
	}
	if err := reopened.UnlockAccess("999999"); err == nil {
		t.Fatal("wrong PIN unlocked")
	}
	replacement, err := reopened.RecoverAccess(code, "654321")
	if err != nil || replacement == code || !reopened.AccessStatus().Locked {
		t.Fatalf("recovery: %v", err)
	}
	if _, err := reopened.RecoverAccess(code, "111111"); err == nil {
		t.Fatal("reused recovery code")
	}
	if err := reopened.UnlockAccess("123456"); err == nil {
		t.Fatal("old PIN unlocked")
	}
	if err := reopened.UnlockAccess("654321"); err != nil {
		t.Fatal(err)
	}
	if err := reopened.DisableAccess("654321"); err != nil {
		t.Fatal(err)
	}
	reopened, _ = New(s.dir, nil, "")
	if reopened.AccessStatus().Enabled || reopened.AccessStatus().Locked {
		t.Fatal("disabled lock returned")
	}
}

func TestApplicationLockAttemptsPersistAndStorageFailureStaysLocked(t *testing.T) {
	s := testService(t, nil)
	enableTestAccess(t, s, AccessPolicy{IdleMinutes: 15})
	now := time.Now()
	s.access.now = func() time.Time { return now }
	_ = s.Lock()
	for i := 0; i < 5; i++ {
		if s.UnlockAccess("000000") == nil {
			t.Fatal("wrong PIN accepted")
		}
	}
	if !s.AccessStatus().RetryAt.Equal(now.Add(30 * time.Second)) {
		t.Fatal("missing delay")
	}
	reopened, _ := New(s.dir, nil, "")
	reopened.access.now = func() time.Time { return now }
	if err := reopened.UnlockAccess("123456"); err == nil {
		t.Fatal("restart bypassed delay")
	}
	now = now.Add(31 * time.Second)
	if err := reopened.UnlockAccess("123456"); err != nil {
		t.Fatal(err)
	}
	_ = reopened.Lock()
	if err := os.Remove(filepath.Join(s.dir, accessFile)); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(s.dir, accessFile), 0700); err != nil {
		t.Fatal(err)
	}
	if err := reopened.UnlockAccess("123456"); err == nil || !reopened.AccessStatus().Locked {
		t.Fatal("failed durable verification unlocked")
	}
}

func TestApplicationLockMissingAndCorruptRecordsFailClosed(t *testing.T) {
	for _, missing := range []bool{true, false} {
		s := testService(t, nil)
		enableTestAccess(t, s, AccessPolicy{})
		if missing {
			_ = os.Remove(filepath.Join(s.dir, accessFile))
		} else {
			_ = os.WriteFile(filepath.Join(s.dir, accessFile), []byte("invalid"), 0600)
		}
		reopened, _ := New(s.dir, nil, "")
		status := reopened.AccessStatus()
		if !status.Locked || status.Error == "" || reopened.Remove("unknown") == nil {
			t.Fatal("unreadable lock allowed management")
		}
	}
}

func TestApplicationLockGuardsEveryManagementMethod(t *testing.T) {
	rpc := &fakeRPC{}
	s := testService(t, rpc)
	s.store.Connections = []Connection{testConnection()}
	enableTestAccess(t, s, AccessPolicy{})
	_ = s.Lock()
	allowed := map[string]bool{"AccessStatus": true, "Snapshot": true, "DesktopVisible": true, "Lock": true, "UnlockAccess": true, "RecoverAccess": true, "Exit": true}
	typ, value := reflect.TypeOf(s), reflect.ValueOf(s)
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		if allowed[method.Name] {
			continue
		}
		t.Run(method.Name, func(t *testing.T) {
			args := []reflect.Value{}
			for i := 1; i < method.Type.NumIn(); i++ {
				args = append(args, reflect.Zero(method.Type.In(i)))
			}
			results := value.MethodByName(method.Name).Call(args)
			if len(results) > 0 && results[len(results)-1].Type() == reflect.TypeOf((*error)(nil)).Elem() {
				last := results[len(results)-1]
				if last.IsNil() || !strings.Contains(last.Interface().(error).Error(), "Unlock SDrive") {
					t.Fatalf("management method not guarded: %v", results)
				}
			}
		})
	}
	if len(rpc.calls) != 0 {
		t.Fatalf("locked management reached engine: %v", rpc.calls)
	}
	v := s.Snapshot()
	b, _ := json.Marshal(v)
	if len(v.Connections) != 0 || v.CacheLocation != "" || v.EngineError != "" || bytes.Contains(b, []byte("Teste")) {
		t.Fatal("locked snapshot exposed profile")
	}
	for _, method := range rpc.calls {
		if method != "mount/listmounts" {
			t.Fatalf("locked management reached engine: %v", rpc.calls)
		}
	}
}

func TestSnapshotAndAccessDetailsCannotReturnAcrossApplicationLock(t *testing.T) {
	for _, details := range []bool{false, true} {
		entered, release := make(chan struct{}), make(chan struct{})
		rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
			if (!details && method == "mount/listmounts") || (details && method == "config/get") {
				close(entered)
				<-release
			}
			if details {
				return map[string]string{"type": "ftp", "host": "private.example.test", "user": "private-user"}, nil
			}
			return map[string]any{"mountPoints": []any{}}, nil
		}}
		s := testService(t, rpc)
		s.store.Connections = []Connection{testConnection()}
		enableTestAccess(t, s, AccessPolicy{})
		finished := make(chan error, 1)
		go func() {
			if details {
				v, err := s.ConnectionFields(s.store.Connections[0].ID)
				if len(v.Values) > 0 {
					err = fmt.Errorf("late access details exposed")
				}
				finished <- err
			} else {
				v := s.Snapshot()
				if len(v.Connections) > 0 {
					finished <- fmt.Errorf("late snapshot exposed")
				} else {
					finished <- nil
				}
			}
		}()
		<-entered
		_ = s.Lock()
		if details {
			if err := s.UnlockAccess("123456"); err != nil {
				t.Fatal(err)
			}
		}
		close(release)
		err := <-finished
		if details && err == nil {
			t.Fatal("access details survived lock/unlock")
		}
		if !details && err != nil {
			t.Fatal(err)
		}
	}
}

func TestIdleReconnectStillRequiresSessionAuthentication(t *testing.T) {
	s := testService(t, &fakeRPC{})
	enableTestAccess(t, s, AccessPolicy{IdleMinutes: 1, DisconnectIdle: true, Reconnect: true})
	c := testConnection()
	c.Provider = "webdav"
	c.SessionOnly = true
	s.store.Connections = []Connection{c}
	s.access.reconnect = []string{c.ID}
	s.mountAvailable = func() bool { return true }
	s.driveUsed = func(string) bool { return false }
	s.startConnection = func(Connection, string) (RPC, error) {
		t.Fatal("reconnect bypassed session authentication")
		return nil, nil
	}
	s.resumeAccess()
	if !strings.Contains(s.errors[c.ID], "Sign in again") || s.mounted[c.ID] || !s.store.Connections[0].Recovery {
		t.Fatal("reconnect bypassed authentication or recovery")
	}
}

func TestLockRetainsUncertainSetupAndAllowsExplicitCancellationRetry(t *testing.T) {
	var attempts atomic.Int32
	var reject atomic.Bool
	reject.Store(true)
	rpc := &fakeRPC{handle: func(_ context.Context, method string, _ any) (any, error) {
		if method == "config/delete" {
			attempts.Add(1)
		}
		if method == "config/delete" && reject.Load() {
			return nil, fmt.Errorf("unavailable")
		}
		return map[string]any{}, nil
	}}
	s := testService(t, rpc)
	enableTestAccess(t, s, AccessPolicy{})
	s.session = &ConfigSession{ID: "retained", Complete: true, oldRemote: "old", rpc: rpc}
	changed := make(chan bool, 8)
	SetAccessChanged(s, func(locked bool) { changed <- locked })
	if err := s.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := s.Lock(); err != nil {
		t.Fatal(err)
	}
	<-changed
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("cancellation failure was not reported")
	}
	if attempts.Load() != 1 {
		t.Fatal("repeated lock duplicated setup cancellation")
	}
	if s.AccessStatus().Warning == "" {
		t.Fatal("uncertain cancellation lost its warning")
	}
	if err := s.UnlockAccess("123456"); err != nil {
		t.Fatal(err)
	}
	if !s.AccessStatus().SetupPending {
		t.Fatal("uncertain setup was discarded")
	}
	reject.Store(false)
	if err := s.EndConfig(""); err != nil {
		t.Fatal(err)
	}
	if s.AccessStatus().SetupPending || s.AccessStatus().Warning != "" {
		t.Fatal("setup retry did not clear the retained reservation")
	}
}

func TestComputerInactivityUsesSafeDisconnectAndPreservesUnknownState(t *testing.T) {
	for _, failed := range []bool{false, true} {
		rpc := &fakeRPC{}
		if failed {
			rpc.handle = func(context.Context, string, any) (any, error) { return nil, fmt.Errorf("unavailable") }
		}
		s := testService(t, rpc)
		c := testConnection()
		s.store.Connections = []Connection{c}
		s.mounted[c.ID] = true
		enableTestAccess(t, s, AccessPolicy{IdleMinutes: 1, DisconnectIdle: true, Reconnect: true})
		s.access.idle = func() (time.Duration, error) { return 2 * time.Minute, nil }
		CheckInactivity(s)
		if !s.AccessStatus().Locked {
			t.Fatal("idle did not lock")
		}
		if failed {
			if !s.mounted[c.ID] || s.access.warning == "" || len(s.access.reconnect) != 0 {
				t.Fatal("unknown state authorized disconnect")
			}
		} else if s.mounted[c.ID] || len(s.access.reconnect) != 1 || s.access.reconnect[0] != c.ID {
			t.Fatal("successful idle disconnect was not recorded")
		}
		before := len(rpc.calls)
		CheckInactivity(s)
		if len(rpc.calls) != before {
			t.Fatal("repeated idle interval repeated disconnection")
		}
	}
}

func TestInactivityFailureAndInvalidPoliciesDoNotDisconnect(t *testing.T) {
	s := testService(t, &fakeRPC{})
	for _, policy := range []AccessPolicy{{IdleMinutes: -1}, {IdleMinutes: 1441}, {DisconnectIdle: true}, {Reconnect: true}} {
		if _, err := s.ConfigureAccess("", "123456", policy); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
	enableTestAccess(t, s, AccessPolicy{IdleMinutes: 1})
	s.access.idle = func() (time.Duration, error) { return 0, fmt.Errorf("unavailable") }
	CheckInactivity(s)
	if s.AccessStatus().Locked {
		t.Fatal("failed idle measurement triggered lock")
	}
}
