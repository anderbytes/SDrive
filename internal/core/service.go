package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"sdrive/internal/diagnostics"
	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

type RPC interface {
	Call(context.Context, string, any, any) error
	Alive() bool
}
type View struct {
	Providers        []Provider       `json:"providers"`
	Connections      []ConnectionView `json:"connections"`
	Settings         Settings         `json:"settings"`
	Letters          []string         `json:"letters"`
	WinFsp           bool             `json:"winFsp"`
	EngineError      string           `json:"engineError"`
	SettingsError    string           `json:"settingsError"`
	BackupAvailable  bool             `json:"backupAvailable"`
	RecoveryHold     bool             `json:"recoveryHold"`
	MaintenanceError string           `json:"maintenanceError"`
	StartupWarning   string           `json:"startupWarning"`
	FreeBytes        *uint64          `json:"freeBytes"`
}
type ConnectionView struct {
	Connection
	Mounted   bool        `json:"mounted"`
	Pending   *int        `json:"pending"`
	Busy      bool        `json:"busy"`
	Operation string      `json:"operation"`
	Error     string      `json:"error"`
	Uploads   *UploadView `json:"uploads"`
	Cache     CacheView   `json:"cache"`
}
type Example struct {
	Value string
	Help  string
}
type Question struct {
	Name        string
	Help        string
	DefaultStr  string
	Type        string
	IsPassword  bool
	Sensitive   bool
	Required    bool
	Exclusive   bool
	Examples    []Example
	Unsupported bool
}
type ConfigResult struct {
	State  string
	Option *Question
	Error  string
}

func (r *ConfigResult) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errInvalidConfigResult
	}
	for _, key := range []string{"State", "Option", "Error"} {
		value, ok := fields[key]
		if !ok || (key != "Option" && string(value) == "null") {
			return errInvalidConfigResult
		}
	}
	type result ConfigResult
	var next result
	if err := json.Unmarshal(data, &next); err != nil {
		return errInvalidConfigResult
	}
	if (next.State != "") != (next.Option != nil) || (next.Option != nil && strings.TrimSpace(next.Option.Name) == "") {
		return errInvalidConfigResult
	}
	*r = ConfigResult(next)
	return nil
}

type ConfigInput struct {
	Connection    Connection        `json:"connection"`
	Values        map[string]string `json:"values"`
	AllowInsecure bool              `json:"allowInsecure"`
	Transport     string            `json:"transport"`
	HostKey       string            `json:"hostKey"`
	Guided        bool              `json:"guided"`
}
type ConfigSession struct {
	ID            string    `json:"id"`
	Busy          bool      `json:"busy"`
	Complete      bool      `json:"complete"`
	Question      *Question `json:"question"`
	Error         string    `json:"error"`
	Authenticated bool      `json:"authenticated"`
	ConnectionID  string    `json:"connectionID"`
	guided        bool
	state         string
	connection    Connection
	oldRemote     string
	inPlace       bool
	cancel        context.CancelFunc
	ctx           context.Context
	stop          context.CancelFunc
	done          chan struct{}
	closing       bool
	rpc           RPC
}
type Service struct {
	mu                   sync.Mutex
	dir                  string
	store                Store
	rpc                  RPC
	settingsError        string
	incompatibleSettings bool
	recoveryHold         bool
	maintenanceError     string
	startupWarning       string
	autoStart            func(bool) error
	winFsp               func() bool
	driveUsed            func(string) bool
	engineError          string
	mounted              map[string]bool
	busy                 map[string]bool
	operations           map[string]string
	errors               map[string]string
	session              *ConfigSession
	exiting              bool
	open                 func(string) error
	quit                 func()
	restart              func() (RPC, error)
	pickFile             func(string) (string, error)
	languageChanged      func(string)
	snapshotID           uint64
	inspections          map[string]*cacheInspection
	desktopVisible       func() bool
	snapshotChanged      func(uint64, View)
	appVersion           string
	diagnostics          *diagnostics.Collector
}

func New(dir string, rpc RPC, engineError string) (*Service, error) {
	store, loadErr := loadStore(dir)
	s := &Service{dir: dir, store: store, rpc: rpc, engineError: engineError, autoStart: platform.AutoStart, winFsp: platform.WinFspInstalled, driveUsed: platform.DriveUsed, mounted: map[string]bool{}, busy: map[string]bool{}, operations: map[string]string{}, errors: map[string]string{}}
	s.initializeRecovery(loadErr)
	if s.settingsError == "" {
		_ = s.cleanup()
		if matches, err := platform.AutoStartMatches(s.store.Settings.AutoStart); err != nil || !matches {
			s.startupWarning = "Windows startup may differ from saved preferences. Save preferences to reconcile it."
		}
	}
	return s, nil
}

func (s *Service) purgeOrphans() error {
	if _, err := os.Stat(filepath.Join(s.dir, recoveryMarker)); !os.IsNotExist(err) {
		s.recoveryHold = true
	}
	if s.settingsError != "" || s.recoveryHold || s.rpc == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(s.dir, "settings.json")); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	// Revalidate disk state before destructive maintenance, including after external changes.
	current, err := loadStore(s.dir)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, store := range []Store{s.store, current} {
		for _, c := range store.Connections {
			keep["sd_"+c.Remote] = true
		}
	}
	backup, err := s.backupStore()
	if err == nil {
		for _, c := range backup.Connections {
			keep["sd_"+c.Remote] = true
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("could not verify settings backup; cleanup is suspended")
	}
	cache, err := s.cacheRemotes()
	if err != nil {
		return err
	}
	remotes, err := s.listRemotes()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, name := range remotes {
		if !generatedRemote(name) || keep[name] || cache[name] {
			continue
		}
		if err := s.call(ctx, "config/delete", map[string]string{"name": name}, nil); err != nil {
			return fmt.Errorf("could not remove an incomplete setup from the encrypted configuration")
		}
	}
	return nil
}

func SetDesktop(s *Service, open func(string) error, quit func(), restart func() (RPC, error)) {
	s.open = open
	s.quit = quit
	s.restart = restart
}

// Caller holds s.mu, or owns the service before publication.
func (s *Service) call(ctx context.Context, method string, in, out any) error {
	if s.rpc == nil {
		return fmt.Errorf("engine unavailable: %s", s.engineError)
	}
	return s.rpc.Call(ctx, method, in, out)
}
func callRPC(ctx context.Context, rpc RPC, method string, in, out any) error {
	if rpc == nil {
		return fmt.Errorf("engine unavailable")
	}
	return rpc.Call(ctx, method, in, out)
}
func (s *Service) index(id string) int {
	for i, c := range s.store.Connections {
		if c.ID == id {
			return i
		}
	}
	return -1
}
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func remote(c Connection) string { return "sd_" + c.Remote + ":" + c.Path }

func (s *Service) Snapshot() View {
	s.mu.Lock()
	s.snapshotID++
	snapshotID := s.snapshotID
	rpc := s.rpc
	_, backupErr := s.backupStore()
	v := View{SettingsError: s.settingsError, BackupAvailable: backupErr == nil && !s.incompatibleSettings, RecoveryHold: s.recoveryHold, MaintenanceError: s.maintenanceError, StartupWarning: s.startupWarning, Providers: Catalog(), Connections: []ConnectionView{}, Settings: s.store.Settings, Letters: []string{}, WinFsp: s.winFsp(), EngineError: s.engineError}
	if s.rpc != nil && !s.rpc.Alive() {
		v.EngineError = "Engine stopped. Restart SDrive to recover your drives."
		if stopped, ok := s.rpc.(interface{ EngineExitSummary() string }); ok {
			if summary := stopped.EngineExitSummary(); summary != "" {
				v.EngineError = summary
			}
		}
	}
	reserved := map[string]bool{}
	for _, c := range s.store.Connections {
		reserved[c.Letter] = true
	}
	for letter := 'D'; letter <= 'Z'; letter++ {
		if !s.driveUsed(string(letter)) && !reserved[string(letter)] {
			v.Letters = append(v.Letters, string(letter))
		}
	}
	for _, c := range s.store.Connections {
		cv := ConnectionView{Connection: c, Mounted: s.mounted[c.ID] && v.EngineError == "", Busy: s.busy[c.ID], Operation: s.operations[c.ID], Error: s.errors[c.ID]}
		v.Connections = append(v.Connections, cv)
	}
	s.mu.Unlock()
	if free, err := platform.FreeBytes(s.dir); err == nil {
		v.FreeBytes = &free
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var mounts []engineMount
	var mountsErr error
	if len(v.Connections) > 0 && rpc != nil && rpc.Alive() {
		mounts, mountsErr = readMounts(ctx, rpc)
	}
	for i := range v.Connections {
		cv := &v.Connections[i]
		s.mu.Lock()
		current := s.index(cv.ID)
		if s.snapshotID == snapshotID && s.rpc == rpc && current >= 0 && s.store.Connections[current] == cv.Connection && !s.busy[cv.ID] && mountsErr == nil && rpc != nil && rpc.Alive() {
			active := false
			for _, m := range mounts {
				if mountLetter(m.MountPoint) == cv.Letter+":" {
					active = m.Fs == remote(cv.Connection)
					if !active {
						cv.Error = "drive letter belongs to a different mount; preserve the cache"
					}
				}
			}
			if s.mounted[cv.ID] && !active {
				cv.Error = "drive was unexpectedly unmounted; use Connect to recover pending files"
			}
			if active && !s.driveUsed(cv.Letter) {
				cv.Error = "engine reports a mount but the Windows drive is unavailable; try Disconnect and preserve the cache"
			}
			if !active && s.driveUsed(cv.Letter) {
				cv.Error = "drive letter is occupied outside this connection; preserve the cache"
			}
			if cv.Error != "" {
				s.errors[cv.ID] = cv.Error
			}
			s.mounted[cv.ID] = active
			cv.Mounted = active
			if active && !cv.Recovery {
				s.store.Connections[current].Recovery = true
				cv.Recovery = true
				if err := saveStore(s.dir, s.store); err != nil {
					cv.Error = "could not save mount recovery state; preserve the data directory"
				}
			}
		}
		s.mu.Unlock()
		var statsErr error
		var used *uint64
		if cv.Mounted {
			cv.Uploads, used, statsErr = readCacheStats(ctx, rpc, cv.Connection)
		}
		sample := s.sampleCache(ctx, cv.Connection)
		cv.Cache = sample.CacheView
		if sample.err == nil {
			count := sample.pending
			cv.Pending = &count
		}
		if sample.err != nil && cv.Cache.Error == "" {
			cv.Cache.Error = "Could not inspect local cache; preserve the files and try again."
		}
		if cv.Mounted {
			cv.Cache.UsedBytes = used
			if statsErr != nil {
				cv.Pending = nil
				cv.Cache.Error = statsErr.Error()
			} else if cv.Pending != nil {
				count := max(*cv.Pending, cv.Uploads.Active+cv.Uploads.Queued, cv.Uploads.CacheErrors)
				cv.Pending = &count
			}
		}
		if mountsErr != nil {
			cv.Pending = nil
			cv.Cache.Error = mountsErr.Error()
		}
		s.mu.Lock()
		inspection := s.inspections[cv.ID]
		if inspection == nil || inspection.result.CheckedAt != cv.Cache.CheckedAt || inspection.connection != cv.Connection || s.rpc != rpc {
			cv.Pending = nil
			cv.Uploads = nil
			cv.Cache = CacheView{Error: "Could not inspect local cache; preserve the files and try again."}
		}
		s.mu.Unlock()
	}
	s.mu.Lock()
	changed := s.snapshotChanged
	current := s.snapshotID == snapshotID && s.rpc == rpc
	s.mu.Unlock()
	if current && changed != nil {
		changed(snapshotID, v)
	}
	return v
}

func (s *Service) BeginConfig(input ConfigInput) (resultID string, resultErr error) {
	defer func() { s.diagnostics.Operation("setup", resultErr) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return "", err
	}
	if s.recoveryHold && (s.index(input.Connection.ID) < 0 || !s.store.Connections[s.index(input.Connection.ID)].Recovery) {
		return "", fmt.Errorf("complete settings recovery before creating or replacing connections")
	}
	if s.exiting {
		return "", fmt.Errorf("exit in progress")
	}
	if s.session != nil {
		return "", fmt.Errorf("complete or cancel the open setup")
	}
	c := input.Connection
	if err := validateConnection(&c); err != nil {
		return "", err
	}
	oldRemote := ""
	inPlace := false
	if c.ID != "" {
		i := s.index(c.ID)
		if i < 0 {
			return "", fmt.Errorf("connection not found")
		}
		old := s.store.Connections[i]
		if s.mounted[c.ID] || s.busy[c.ID] {
			return "", fmt.Errorf("unmount before reconfiguring")
		}
		oldRemote = old.Remote
		if old.Recovery {
			if c.Provider != old.Provider || c.Path != old.Path || c.Letter != old.Letter || c.ReadOnly != old.ReadOnly {
				return "", fmt.Errorf("preserve provider, folder, letter and access mode during recovery")
			}
			c = old
			inPlace = true
		}
	} else {
		c.ID = newID()
	}
	for _, existing := range s.store.Connections {
		if existing.ID != c.ID && existing.Letter == c.Letter {
			return "", fmt.Errorf("letter already reserved by another connection")
		}
	}
	p, _ := provider(c.Provider)
	params, err := parameters(p, input.Values, input.AllowInsecure, input.Transport)
	if err != nil {
		return "", err
	}
	sessionCtx, stop := context.WithCancel(context.Background())
	ctx, cancel := context.WithTimeout(sessionCtx, 10*time.Minute)
	ss := &ConfigSession{ID: newID(), Busy: true, connection: c, ConnectionID: c.ID, cancel: cancel, ctx: sessionCtx, stop: stop, rpc: s.rpc, done: make(chan struct{}), inPlace: inPlace, guided: input.Guided}
	s.session = ss
	done := ss.done
	started := false
	defer func() {
		if !started {
			stop()
			cancel()
			close(done)
			if s.session == ss {
				s.session = nil
			}
		}
	}()
	getConfig := func(name string, out any) error {
		getCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		s.mu.Unlock()
		err := callRPC(getCtx, ss.rpc, "config/get", map[string]string{"name": name}, out)
		s.mu.Lock()
		if s.session != ss || ss.closing || ctx.Err() != nil {
			return fmt.Errorf("setup changed; try again")
		}
		return err
	}
	if input.Guided {
		if err = oauthParameters(p, params); err != nil {
			if oldRemote == "" {
				return "", err
			}
			var existing map[string]string
			if getErr := getConfig("sd_"+oldRemote, &existing); getErr != nil || existing["client_id"] == "" {
				return "", err
			}
			params["client_id"], params["client_secret"] = existing["client_id"], existing["client_secret"]
		}
	}
	if p.ID == "sftp" {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(input.HostKey))
		if err != nil {
			return "", fmt.Errorf("verify and accept the server's SSH key before saving")
		}
		params["host_keys"] = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
	}
	if p.ID == "drive" {
		if c.ReadOnly {
			params["scope"] = "drive.readonly"
		} else {
			params["scope"] = "drive"
		}
	}
	params["config_is_local"] = "true"
	if inPlace {
		var existing map[string]string
		if err = getConfig("sd_"+c.Remote, &existing); err != nil {
			return "", err
		}
		if existing == nil || existing["type"] != p.Backend {
			return "", fmt.Errorf("invalid saved access details; preserve the connection and try again")
		}
		if p.ID == "ftp" {
			mode, modeErr := ftpTransport(existing)
			if modeErr != nil {
				return "", modeErr
			}
			if mode != input.Transport {
				return "", fmt.Errorf("keep the FTP transport during recovery")
			}
		}
		for key, value := range params {
			if key == "sas_url" {
				next, _ := url.Parse(value)
				old, _ := url.Parse(existing[key])
				if next == nil || old == nil || next.Host != old.Host || next.Path != old.Path {
					return "", fmt.Errorf("the SAS URL must keep the destination during recovery")
				}
			}
			if key == "config_is_local" || key == "client_id" || key == "client_secret" || key == "scope" || key == "host_keys" || key == "access_key_id" || key == "service_account_file" || (key == "account" && p.ID == "b2") {
				continue
			}
			secret := false
			for _, f := range p.Fields {
				if f.Key == key && f.Secret {
					secret = true
				}
			}
			if !secret && !(p.ID == "ftp" && (key == "tls" || key == "explicit_tls")) && value != existing[key] {
				return "", fmt.Errorf("during recovery only renew credentials, keeping the server and username")
			}
		}
		delete(params, "host_keys")
		oldRemote = ""
	} else {
		c.Remote = newID()
		c.Recovery = false
		c.TargetHash = ""
	}
	delete(s.inspections, c.ID)
	ss.connection, ss.oldRemote = c, oldRemote
	id := ss.ID
	started = true
	method := "config/create"
	if inPlace {
		method = "config/update"
	}
	go func() {
		defer cancel()
		defer close(done)
		s.configure(ctx, id, ss.rpc, method, map[string]any{"name": "sd_" + c.Remote, "type": p.Backend, "parameters": params, "opt": map[string]any{"nonInteractive": true, "obscure": true}})
	}()
	return id, nil
}

func (s *Service) configure(ctx context.Context, id string, rpc RPC, method string, input map[string]any) {
	var result ConfigResult
	err := callRPC(ctx, rpc, method, input, &result)
	var config map[string]string
	if err == nil && result.State == "" && result.Option == nil && result.Error == "" {
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = callRPC(verifyCtx, rpc, "config/get", map[string]any{"name": input["name"]}, &config)
		cancel()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.ID != id || s.session.closing {
		return
	}
	ss := s.session
	ss.Busy = false
	defer func() {
		failure := err
		if failure == nil && ss.Error != "" {
			failure = fmt.Errorf("setup failed")
		}
		s.diagnostics.Operation("setup", failure)
	}()
	if ctx.Err() != nil {
		ss.Error = "Setup expired; cancel and try again"
		return
	}
	if err != nil {
		ss.Error = setupFailure(err)
		return
	}
	ss.state = result.State
	ss.Question = result.Option
	if q := ss.Question; q != nil && sensitiveQuestion(q.Name) {
		q.Sensitive = true
	}
	if ss.Question != nil && (ss.Question.IsPassword || ss.Question.Sensitive) {
		ss.Question.DefaultStr = ""
		ss.Question.Examples = nil
	}
	if ss.guided && ss.Question != nil {
		localizeQuestion(ss.Question)
	}
	if result.Error != "" {
		ss.Error = "The provider did not accept the answer. Check the details and try again."
		return
	}
	ss.Error = ""
	if result.State == "" && result.Option == nil {
		p, _ := provider(ss.connection.Provider)
		if err := validateConfiguredRemote(p, config); err != nil {
			ss.Error = err.Error()
			return
		}
		if ss.inPlace {
			if _, err := checkDestination(ss.connection, config); err != nil {
				ss.Error = err.Error()
				return
			}
		}
		ss.Authenticated = true
		if !ss.guided {
			ss.Error = s.commitConfig(ss)
		}
	}
}

// Caller holds s.mu; cancellation must never remove a committed remote.
func (s *Service) commitConfig(ss *ConfigSession) string {
	for _, other := range s.store.Connections {
		if other.ID != ss.connection.ID && other.Letter == ss.connection.Letter {
			return "The chosen letter was reserved by another connection. Cancel and choose another letter."
		}
	}
	previous := append([]Connection{}, s.store.Connections...)
	if i := s.index(ss.connection.ID); i >= 0 {
		s.store.Connections[i] = ss.connection
	} else {
		s.store.Connections = append(s.store.Connections, ss.connection)
	}
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.Connections = previous
		return "Could not save preferences; cancel and try again"
	}
	ss.Complete = true
	delete(s.inspections, ss.connection.ID)

	if ss.oldRemote != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		name := ss.oldRemote
		ss.Busy = true
		s.mu.Unlock()
		err := callRPC(ctx, ss.rpc, "config/delete", map[string]string{"name": "sd_" + name}, nil)
		s.mu.Lock()
		ss.Busy = false
		if err != nil {
			return "Connection saved; old credential cleanup failed. Try Done again."
		}
		ss.oldRemote = ""
	}

	return ""
}

func (s *Service) ConfigStatus(id string) (ConfigSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session == nil || s.session.ID != id {
		return ConfigSession{}, fmt.Errorf("setup not found")
	}
	return *s.session, nil
}
func (s *Service) AnswerConfig(id, answer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss := s.session
	if ss == nil || ss.ID != id || ss.closing || ss.Busy || ss.Complete || ss.Question == nil {
		return fmt.Errorf("setup is not waiting for an answer")
	}
	if len(answer) > 65536 || strings.ContainsRune(answer, 0) {
		return fmt.Errorf("invalid answer")
	}
	ss.Busy = true
	ss.Error = ""
	ctx, cancel := context.WithTimeout(ss.ctx, 10*time.Minute)
	ss.cancel = cancel
	ss.done = make(chan struct{})
	done := ss.done
	input := map[string]any{"name": "sd_" + ss.connection.Remote, "parameters": map[string]string{}, "opt": map[string]any{"nonInteractive": true, "continue": true, "state": ss.state, "result": answer, "obscure": true}}
	go func() { defer cancel(); defer close(done); s.configure(ctx, id, ss.rpc, "config/update", input) }()
	return nil
}
func (s *Service) EndConfig(id string) (resultErr error) {
	defer func() { s.diagnostics.Operation("setup", resultErr) }()
	s.mu.Lock()
	ss := s.session
	if ss == nil || ss.ID != id {
		s.mu.Unlock()
		return nil
	}
	ss.closing = true
	if ss.stop != nil {
		ss.stop()
	}
	if ss.cancel != nil {
		ss.cancel()
	}
	rpc, done, complete := ss.rpc, ss.done, ss.Complete
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if rpc != nil && rpc.Alive() && !complete {
		_ = rpc.Call(ctx, "config/oauthstop", map[string]any{}, nil)
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return fmt.Errorf("setup is still stopping; try Cancel again")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != ss {
		return nil
	}
	name := ss.oldRemote
	if !ss.Complete && !ss.inPlace {
		name = ss.connection.Remote
	}
	s.mu.Unlock()
	var cleanupErr error
	if rpc != nil && rpc.Alive() && name != "" {
		cleanupErr = rpc.Call(ctx, "config/delete", map[string]string{"name": "sd_" + name}, nil)
	}
	s.mu.Lock()
	if cleanupErr != nil {
		return cleanupErr
	}
	if s.session != ss {
		return nil
	}
	s.session = nil
	return nil
}

type HostIdentity struct {
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint"`
}

func (s *Service) ProbeSSH(host, port string) (HostIdentity, error) {
	if err := validateServer(host, port); err != nil {
		return HostIdentity{}, err
	}
	host = strings.Trim(host, "[]")
	if port == "" {
		port = "22"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 8*time.Second)
	if err != nil {
		return HostIdentity{}, fmt.Errorf("could not reach the SSH server")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))
	identity := HostIdentity{}
	config := &ssh.ClientConfig{User: "sdrive-host-verification", HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		identity = HostIdentity{Key: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), Fingerprint: ssh.FingerprintSHA256(key)}
		return fmt.Errorf("host key collected before authentication")
	}}
	_, _, _, _ = ssh.NewClientConn(conn, net.JoinHostPort(host, port), config)
	if identity.Key == "" {
		return identity, fmt.Errorf("could not retrieve the SSH key")
	}
	return identity, nil
}

func (s *Service) UpdateConnection(c Connection) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil && s.session.connection.ID == c.ID {
		return fmt.Errorf("connection operation in progress")
	}
	return s.updateConnection(c)
}

// Caller holds s.mu; completed setup may reuse this for a letter-only correction.
func (s *Service) updateConnection(c Connection) error {
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.recoveryHold {
		return fmt.Errorf("complete settings recovery before editing or removing connections")
	}
	if err := validateConnection(&c); err != nil {
		return err
	}
	i := s.index(c.ID)
	if i < 0 {
		return fmt.Errorf("connection not found")
	}
	old := s.store.Connections[i]
	if s.exiting {
		return fmt.Errorf("connection operation in progress")
	}
	if s.mounted[c.ID] || s.busy[c.ID] || old.Recovery {
		return fmt.Errorf("unmount and resolve pending files before editing")
	}
	if c.Provider != old.Provider {
		return fmt.Errorf("use Reconfigure to change the provider")
	}
	if c.Provider == "drive" && c.ReadOnly != old.ReadOnly {
		return fmt.Errorf("use Reconfigure account to change access mode and update Google Drive OAuth consent")
	}
	for _, other := range s.store.Connections {
		if other.ID != c.ID && other.Letter == c.Letter {
			return fmt.Errorf("letter already reserved")
		}
	}
	if c.Letter != old.Letter {
		if err := s.availableLetter(c); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pending, err := s.cachedPending(ctx, old)
		if err != nil || pending != 0 {
			return fmt.Errorf("unmount and resolve pending files before editing")
		}
	}
	c.Remote = old.Remote
	c.TargetHash = old.TargetHash
	c.Recovery = false
	s.store.Connections[i] = c
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.Connections[i] = old
		return err
	}
	delete(s.inspections, c.ID)
	s.snapshotID++
	return nil
}
func (s *Service) Remove(id string) error {
	s.mu.Lock()
	delete(s.inspections, id)
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.recoveryHold {
		return fmt.Errorf("complete settings recovery before editing or removing connections")
	}
	i := s.index(id)
	if i < 0 {
		return fmt.Errorf("connection not found")
	}
	c := s.store.Connections[i]
	if s.exiting {
		return fmt.Errorf("exit in progress")
	}
	if s.session != nil && s.session.connection.ID == id {
		return fmt.Errorf("complete or cancel reconfiguration")
	}
	if s.mounted[id] || s.busy[id] || c.Recovery {
		return fmt.Errorf("unmount and resolve pending files before removing")
	}
	previous := append([]Connection{}, s.store.Connections...)
	s.store.Connections = append(s.store.Connections[:i], s.store.Connections[i+1:]...)
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.Connections = previous
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.call(ctx, "config/delete", map[string]string{"name": "sd_" + c.Remote}, nil); err != nil {
		s.store.Connections = previous
		if saveErr := saveStore(s.dir, s.store); saveErr != nil {
			return fmt.Errorf("failed to remove credentials and restore settings; preserve the data directory")
		}
		return err
	}
	return nil
}

type ConnectionFieldsView struct {
	Values    map[string]string `json:"values"`
	Transport string            `json:"transport"`
}

func (s *Service) ConnectionFields(id string) (ConnectionFieldsView, error) {
	s.mu.Lock()
	i := s.index(id)
	if i < 0 {
		s.mu.Unlock()
		return ConnectionFieldsView{}, fmt.Errorf("connection not found")
	}
	c := s.store.Connections[i]
	rpc := s.rpc
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var config map[string]string
	if err := callRPC(ctx, rpc, "config/get", map[string]string{"name": "sd_" + c.Remote}, &config); err != nil {
		return ConnectionFieldsView{}, fmt.Errorf("could not load access details; cancel and try again")
	}
	s.mu.Lock()
	current := s.index(id)
	valid := s.rpc == rpc && current >= 0 && s.store.Connections[current] == c
	s.mu.Unlock()
	if !valid {
		return ConnectionFieldsView{}, fmt.Errorf("connection changed; try again")
	}
	p, _ := provider(c.Provider)
	if config == nil || config["type"] != p.Backend {
		return ConnectionFieldsView{}, fmt.Errorf("invalid saved access details; preserve the connection and try again")
	}
	values := map[string]string{}
	for _, f := range p.Fields {
		if !f.Secret {
			if value, ok := config[f.Key]; ok {
				values[f.Key] = value
			}
		}
	}
	view := ConnectionFieldsView{Values: values}
	if p.ID == "ftp" {
		mode, err := ftpTransport(config)
		if err != nil {
			return ConnectionFieldsView{}, err
		}
		view.Transport = mode
	}
	return view, nil
}

// Language reads only preferences, without querying drives or the engine.
func Language(s *Service) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.store.Settings.Language
}

func SetLanguageChanged(s *Service, changed func(string)) { s.languageChanged = changed }

func (s *Service) SaveSettings(settings Settings) error {
	if settings.Language == "" {
		settings.Language = "en"
	}
	if !i18n.Valid(settings.Language) {
		return fmt.Errorf("unsupported language")
	}
	if settings.CacheGiB < 1 || settings.CacheGiB > 1024 {
		return fmt.Errorf("cache must be between 1 and 1024 GiB per connection")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	old := s.store.Settings
	if s.exiting {
		return fmt.Errorf("exit in progress")
	}
	if err := s.setAutoStart(settings.AutoStart); err != nil {
		return err
	}
	s.store.Settings = settings
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.Settings = old
		if rollbackErr := s.setAutoStart(old.AutoStart); rollbackErr != nil {
			s.startupWarning = "Windows startup may differ from saved preferences. Save preferences to reconcile it."
			return fmt.Errorf("could not save preferences: %w; Windows startup rollback also failed: %v", err, rollbackErr)
		}
		return err
	}
	s.startupWarning = ""
	if s.languageChanged != nil {
		s.languageChanged(settings.Language)
	}
	return nil
}
func (s *Service) TestConnection(id string) error {
	s.mu.Lock()
	i := s.index(id)
	if i < 0 {
		s.mu.Unlock()
		return fmt.Errorf("connection not found")
	}
	if s.exiting || s.busy[id] || (s.session != nil && s.session.connection.ID == id && !s.session.Complete) {
		s.mu.Unlock()
		return fmt.Errorf("connection operation in progress")
	}
	c := s.store.Connections[i]
	rpc := s.rpc
	s.busy[id] = true
	s.operations[id] = "validating"
	s.snapshotID++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.busy[id] = false
		delete(s.operations, id)
		s.snapshotID++
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var result struct{ List []struct{ Name string } }
	if err := callRPC(ctx, rpc, "operations/list", map[string]any{"fs": remote(c), "remote": "", "opt": map[string]bool{"dirsOnly": true, "recurse": false}}, &result); err != nil {
		return err
	}
	s.mu.Lock()
	current := s.index(id)
	valid := s.rpc == rpc && current >= 0 && s.store.Connections[current] == c
	s.mu.Unlock()
	if !valid {
		return fmt.Errorf("connection changed; try again")
	}
	if result.List == nil {
		return fmt.Errorf("provider did not return a valid listing")
	}
	return nil
}
func (s *Service) OpenExplorer(id string) error {
	s.mu.Lock()
	i := s.index(id)
	if i < 0 || !s.mounted[id] {
		s.mu.Unlock()
		return fmt.Errorf("drive is not mounted")
	}
	letter := s.store.Connections[i].Letter
	s.mu.Unlock()
	if s.open == nil {
		return fmt.Errorf("desktop interface unavailable")
	}
	return s.open(letter + `:\`)
}
func (s *Service) Prerequisite(which string) error {
	urls := map[string]string{"winfsp": "https://winfsp.dev/rel/", "webview2": "https://developer.microsoft.com/microsoft-edge/webview2/", "oauth-drive": "https://rclone.org/drive/#making-your-own-client-id"}
	url, ok := urls[which]
	if !ok || s.open == nil {
		return fmt.Errorf("unknown link")
	}
	return s.open(url)
}

func (s *Service) RestartEngine() (resultErr error) {
	defer func() { s.diagnostics.Operation("engine_start", resultErr) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persistenceReady(); err != nil {
		return err
	}
	if s.exiting {
		return fmt.Errorf("exit in progress")
	}
	if s.rpc != nil && s.rpc.Alive() {
		return fmt.Errorf("the engine is active; unmount drives before restarting the application")
	}
	if s.session != nil {
		return fmt.Errorf("cancel setup before restarting the engine")
	}
	for _, busy := range s.busy {
		if busy {
			return fmt.Errorf("wait for operations to finish")
		}
	}
	if s.restart == nil {
		return fmt.Errorf("restart unavailable")
	}
	rpc, err := s.restart()
	if err != nil {
		s.engineError = err.Error()
		return err
	}
	s.rpc = rpc
	s.engineError = ""
	s.mounted = map[string]bool{}
	s.inspections = nil
	s.errors = map[string]string{}
	_ = s.cleanup()
	return nil
}

func AutoMount(s *Service) {
	s.mu.Lock()
	if s.settingsError != "" || s.recoveryHold {
		s.mu.Unlock()
		return
	}
	connections := append([]Connection{}, s.store.Connections...)
	s.mu.Unlock()
	for _, c := range connections {
		if c.AutoMount {
			_ = s.Mount(c.ID)
		}
	}
}
func (s *Service) dataPath() string { return filepath.Join(s.dir, "cache") }

func (s *Service) setAutoStart(enabled bool) error {
	if s.autoStart != nil {
		return s.autoStart(enabled)
	}
	return platform.AutoStart(enabled)
}
