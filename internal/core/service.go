package core

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"sdrive/internal/diagnostics"
	"sdrive/internal/i18n"
	"sdrive/internal/platform"
)

type RPC interface {
	Call(context.Context, string, any, any) error
	Alive() bool
}
type View struct {
	Platform         string           `json:"platform"`
	MountAvailable   bool             `json:"mountAvailable"`
	Access           AccessStatus     `json:"access"`
	DesktopWarning   string           `json:"desktopWarning"`
	ImportDrafts     []ImportDraft    `json:"importDrafts"`
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
	CacheLocation    string           `json:"cacheLocation"`
}
type ConnectionView struct {
	Connection
	ServerPath        string                `json:"serverPath,omitempty"`
	TransportSecurity TransportSecurityView `json:"transportSecurity"`
	Mounted           bool                  `json:"mounted"`
	Pending           *int                  `json:"pending"`
	Busy              bool                  `json:"busy"`
	Operation         string                `json:"operation"`
	Error             string                `json:"error"`
	ErrorMessage      *i18n.Message         `json:"errorMessage,omitempty"`
	Uploads           *UploadView           `json:"uploads"`
	Cache             CacheView             `json:"cache"`
	Capacity          CapacityView          `json:"capacity"`
	Network           NetworkView           `json:"network"`
}
type Service struct {
	networkSessions      map[string]*networkSession
	serverPaths          map[string]*serverPathInspection
	pickAccessFile       func(string) (string, error)
	fileAccessBusy       bool
	access               accessProtection
	desktopMu            sync.Mutex
	syncDriveIcons       func([]Connection) error
	syncDriveLabels      func([]Connection) error
	desktopWarning       string
	explorerWarning      string
	pickPortable         func(bool) (string, error)
	importPreview        *ImportPreview
	mu                   sync.Mutex
	dir                  string
	cacheRoot            string
	store                Store
	rpc                  RPC
	workers              map[string]RPC
	starting             map[string]bool
	startConnection      func(Connection, string) (RPC, error)
	settingsError        string
	incompatibleSettings bool
	recoveryHold         bool
	maintenanceError     string
	startupWarning       string
	autoStart            func(bool) error
	mountAvailable       func() bool
	driveUsed            func(string) bool
	engineError          string
	mounted              map[string]bool
	busy                 map[string]bool
	operations           map[string]string
	errors               map[string]string
	localizedErrors      map[string]*i18n.Message
	session              *ConfigSession
	exiting              bool
	open                 func(string) error
	quit                 func()
	restart              func() (RPC, error)
	pickFile             func(string) (string, error)
	pickDirectory        func() (string, error)
	languageChanged      func(string)
	snapshotID           uint64
	inspections          map[string]*cacheInspection
	capacities           map[string]*capacityInspection
	transportInspections map[string]*transportInspection
	desktopVisible       func() bool
	snapshotChanged      func(uint64, View)
	disconnectChanged    func(DisconnectProgress)
	disconnectSequence   uint64
	releaseDate          string
	pickDiagnosticReport func() (string, error)
	appVersion           string
	diagnostics          *diagnostics.Collector
}

func New(dir string, rpc RPC, engineError string) (*Service, error) {
	store, loadErr := loadStore(dir)
	s := &Service{dir: dir, store: store, rpc: rpc, engineError: engineError, autoStart: platform.AutoStart, mountAvailable: platform.MountAvailable, driveUsed: platform.DriveUsed, mounted: map[string]bool{}, busy: map[string]bool{}, operations: map[string]string{}, errors: map[string]string{}}
	if location, err := readCacheLocation(dir); err == nil {
		s.cacheRoot = location.Path
	} else if !os.IsNotExist(err) {
		loadErr = err
	}
	s.loadAccess()
	s.initializeRecovery(loadErr)
	if err := s.cacheReady(); err != nil {
		s.settingsError = err.Error()
	}
	if s.settingsError == "" && !s.access.locked {
		_ = s.cleanup()
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

func (s *Service) Snapshot() (v View) {
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		v.Access = s.accessStatus()
		if s.access.locked {
			v = s.lockedView()
		}
	}()
	defer s.reconcileDesktopAppearance()
	s.mu.Lock()
	s.snapshotID++
	snapshotID := s.snapshotID
	rpc := s.rpc
	_, backupErr := s.backupStore()
	v = View{SettingsError: s.settingsError, BackupAvailable: backupErr == nil && !s.incompatibleSettings, RecoveryHold: s.recoveryHold, MaintenanceError: s.maintenanceError, StartupWarning: s.startupWarning, Providers: Catalog(), Connections: []ConnectionView{}, Settings: s.store.Settings, Letters: []string{}, WinFsp: s.mountAvailable(), EngineError: s.engineError}
	v.Platform, v.MountAvailable = platform.OS, s.mountAvailable()
	v.WinFsp = platform.OS == "windows" && v.MountAvailable
	v.CacheLocation = s.dataPath()
	v.ImportDrafts = append([]ImportDraft{}, s.store.ImportDrafts...)
	v.DesktopWarning = s.desktopWarnings()
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
		if platform.OS == "windows" && !s.driveUsed(string(letter)) && !reserved[string(letter)] {
			v.Letters = append(v.Letters, string(letter))
		}
	}
	for _, c := range s.store.Connections {
		cv := ConnectionView{Connection: c, Mounted: s.mounted[c.ID] && s.connectionRPC(c) != nil && s.connectionRPC(c).Alive(), Busy: s.busy[c.ID], Operation: s.operations[c.ID], Error: s.errors[c.ID], ErrorMessage: s.localizedErrors[c.ID]}
		v.Connections = append(v.Connections, cv)
	}
	s.mu.Unlock()
	if free, err := platform.FreeBytes(v.CacheLocation); err == nil {
		v.FreeBytes = &free
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var checks sync.WaitGroup
	for i := range v.Connections {
		checks.Add(1)
		go func(i int) {
			defer checks.Done()
			cv := &v.Connections[i]
			cv.ServerPath = s.connectionServerPath(ctx, cv.Connection)
			s.mu.Lock()
			connectionRPC := s.connectionRPC(cv.Connection)
			hasWorker := s.workers[cv.Remote] != nil
			s.mu.Unlock()
			var mounts []engineMount
			var mountsErr error
			if connectionRPC != nil && connectionRPC.Alive() && (s.startConnection == nil || hasWorker) {
				mounts, mountsErr = readMounts(ctx, connectionRPC)
			}
			if hasWorker && !connectionRPC.Alive() {
				cv.Mounted = false
				cv.Error = "Connection engine stopped. Reconnect to recover your files."
				mountsErr = fmt.Errorf("could not verify mounted drives; preserve the cache and try again")
			}
			s.mu.Lock()
			current := s.index(cv.ID)
			if hasWorker && !connectionRPC.Alive() && s.connectionRPC(cv.Connection) == connectionRPC {
				s.mounted[cv.ID] = false
			}
			if s.snapshotID == snapshotID && s.rpc == rpc && s.connectionRPC(cv.Connection) == connectionRPC && current >= 0 && s.store.Connections[current] == cv.Connection && !s.busy[cv.ID] && mountsErr == nil && connectionRPC != nil && connectionRPC.Alive() {
				active := false
				for _, m := range mounts {
					if mountKey(m.MountPoint) == mountKey(mountPoint(cv.Connection)) {
						active = m.Fs == remote(cv.Connection)
						if !active {
							cv.Error = "Mount destination belongs to a different mount; preserve the cache."
						}
					}
				}
				if s.mounted[cv.ID] && !active {
					cv.Error = "drive was unexpectedly unmounted; use Connect to recover pending files"
				}
				if active && !s.mountPresent(cv.Connection) {
					cv.Error = "Engine reports a mount but the destination is unavailable; try Disconnect and preserve the cache."
				}
				if !active && s.mountPresent(cv.Connection) {
					cv.Error = "Mount destination is occupied outside this connection; preserve the cache."
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
				cv.Uploads, used, statsErr = readCacheStats(ctx, connectionRPC, cv.Connection)
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
			if inspection == nil || inspection.result.CheckedAt != cv.Cache.CheckedAt || inspection.connection != cv.Connection || s.rpc != rpc || s.connectionRPC(cv.Connection) != connectionRPC {
				cv.Pending = nil
				cv.Uploads = nil
				cv.Cache = CacheView{Error: "Could not inspect local cache; preserve the files and try again."}
				cv.Capacity = CapacityView{}
			}
			s.mu.Unlock()
			if cv.ErrorMessage != nil && cv.ErrorMessage.Message != cv.Error {
				cv.ErrorMessage = nil
			}
			if cv.Mounted {
				cv.Capacity = s.sampleCapacity(ctx, cv.Connection, connectionRPC)
			}
			cv.TransportSecurity = TransportSecurityView{State: "disconnected"}
			if cv.Mounted {
				cv.TransportSecurity = s.sampleTransportSecurity(ctx, cv.Connection, connectionRPC)
			}
			cv.Network = s.sampleNetwork(ctx, cv.Connection, connectionRPC)
		}(i)
	}
	checks.Wait()
	s.mu.Lock()
	for i := range v.Connections {
		cv := &v.Connections[i]
		if session := s.networkSessions[cv.ID]; session != nil && session.remote == cv.Remote {
			cv.Network = session.view
		} else {
			cv.Network = NetworkView{}
		}
		inspection := s.transportInspections[cv.ID]
		current := s.index(cv.ID)
		if !s.mounted[cv.ID] {
			cv.TransportSecurity = TransportSecurityView{State: "disconnected"}
		} else if inspection == nil || current < 0 || s.store.Connections[current] != cv.Connection || inspection.view.CheckedAt != cv.TransportSecurity.CheckedAt || inspection.rpc != s.connectionRPC(cv.Connection) || !inspection.rpc.Alive() || s.busy[cv.ID] {
			cv.TransportSecurity = TransportSecurityView{State: "unknown"}
		}
	}
	v.Access = s.accessStatus()
	if s.access.locked {
		v = s.lockedView()
	}
	changed := s.snapshotChanged
	current := s.snapshotID == snapshotID && s.rpc == rpc
	s.mu.Unlock()
	if current && changed != nil {
		changed(snapshotID, v)
	}
	return v
}

func (s *Service) UpdateConnection(c Connection) error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	defer s.mu.Unlock()
	if s.session != nil && s.session.connection.ID == c.ID {
		return fmt.Errorf("connection operation in progress")
	}
	return s.updateConnection(c)
}

// Caller holds s.mu; completed setup may reuse this to correct a mount destination.
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
	c.CardStyle, c.IconImage = old.CardStyle, old.IconImage
	if s.exiting {
		return fmt.Errorf("connection operation in progress")
	}
	if s.mounted[c.ID] || s.busy[c.ID] || old.Recovery {
		return fmt.Errorf("unmount and resolve pending files before editing")
	}
	if c.Provider != old.Provider {
		return fmt.Errorf("Create a new connection to use another provider.")
	}
	if c.Provider == "drive" && c.ReadOnly != old.ReadOnly {
		return fmt.Errorf("Use the Access section to change access mode and update Google Drive OAuth consent.")
	}
	for _, other := range s.store.Connections {
		if other.ID != c.ID && mountTargetsOverlap(other, c) {
			field := "letter"
			if c.MountFolder != "" {
				field = "mountFolder"
			}
			return &FieldError{Field: field, Message: "The mount destination is reserved by another connection."}
		}
	}
	if mountTarget(c) != mountTarget(old) {
		if err := s.availableMount(c); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pending, err := s.cachedPending(ctx, old)
		if err != nil || pending != 0 {
			return fmt.Errorf("unmount and resolve pending files before editing")
		}
	}
	if c.SessionOnly != old.SessionOnly || c.AuthMethod != old.AuthMethod || c.AlternateURL != old.AlternateURL || ((old.SessionOnly || old.AlternateURL != "") && c.Path != old.Path) {
		return fmt.Errorf("Use Renew access in the Access section to apply authentication changes.")
	}
	s.stopConnection(old)
	c.Remote = old.Remote
	c.TargetHash = old.TargetHash
	c.PrimaryURL = old.PrimaryURL
	c.ResourceID = old.ResourceID
	c.Binding = old.Binding
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
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
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
	if err := s.forgetRemote(c.Remote); err != nil {
		s.store.Connections = previous
		if saveErr := saveStore(s.dir, s.store); saveErr != nil {
			return fmt.Errorf("failed to remove credentials and restore settings; preserve the data directory")
		}
		return err
	}
	delete(s.errors, id)
	delete(s.localizedErrors, id)
	return nil
}

func (s *Service) TestConnection(id string) error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
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
	if c.SessionOnly && s.workers[c.Remote] == nil && s.startConnection != nil {
		s.mu.Unlock()
		return fmt.Errorf("Sign in again before connecting this session-only drive.")
	}
	s.busy[id] = true
	if err := s.ensureConnection(c); err != nil {
		s.busy[id] = false
		s.mu.Unlock()
		return err
	}
	rpc := s.connectionRPC(c)
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
	valid := s.connectionRPC(c) == rpc && current >= 0 && s.store.Connections[current] == c
	s.mu.Unlock()
	if !valid {
		return fmt.Errorf("connection changed; try again")
	}
	if result.List == nil {
		return fmt.Errorf("provider did not return a valid listing")
	}
	return nil
}
func (s *Service) OpenMount(id string) error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	i := s.index(id)
	if i < 0 || !s.mounted[id] {
		s.mu.Unlock()
		return fmt.Errorf("drive is not mounted")
	}
	target := mountPoint(s.store.Connections[i])
	if s.store.Connections[i].MountFolder == "" {
		target += `\`
	}
	s.mu.Unlock()
	if s.open == nil {
		return fmt.Errorf("desktop interface unavailable")
	}
	err := s.open(target)
	if err == nil {
		s.mu.Lock()
		s.explorerWarning = ""
		s.mu.Unlock()
	}
	return err
}
func (s *Service) Prerequisite(which string) error {
	if err := s.accessAllowed(); err != nil {
		return err
	}
	urls := map[string]string{"winfsp": "https://winfsp.dev/rel/", "webview2": "https://developer.microsoft.com/microsoft-edge/webview2/", "oauth-drive": "https://rclone.org/drive/#making-your-own-client-id", "oauth-onedrive": "https://rclone.org/onedrive/#getting-your-own-client-id-and-key", "oauth-sharepoint": "https://rclone.org/onedrive/#getting-your-own-client-id-and-key", "oauth-dropbox": "https://rclone.org/dropbox/#get-your-own-dropbox-app-id", "oauth-box": "https://rclone.org/box/#get-your-own-box-app-id", "oauth-pcloud": "https://rclone.org/pcloud/", "oauth-gcs": "https://rclone.org/googlecloudstorage/", "oauth-yandex": "https://yandex.com/dev/id/doc/en/register-client"}
	url, ok := urls[which]
	if !ok || s.open == nil {
		return fmt.Errorf("unknown link")
	}
	return s.open(url)
}

func (s *Service) dataPath() string {
	if s.cacheRoot != "" {
		return s.cacheRoot
	}
	return filepath.Join(s.dir, "cache")
}

func (s *Service) recordConnectionError(id string, err error) {
	s.errors[id] = err.Error()
	if s.localizedErrors == nil {
		s.localizedErrors = map[string]*i18n.Message{}
	}
	s.localizedErrors[id] = i18n.MessageOf(err)
}
