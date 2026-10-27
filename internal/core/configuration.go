package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

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
	ImportDraftID     string            `json:"importDraftID,omitempty"`
	Connection        Connection        `json:"connection"`
	Values            map[string]string `json:"values"`
	AllowInsecure     bool              `json:"allowInsecure"`
	Transport         string            `json:"transport"`
	HostKey           string            `json:"hostKey"`
	Guided            bool              `json:"guided"`
	ForgetCredentials bool              `json:"forgetCredentials,omitempty"`
}
type ConfigSession struct {
	importDraftID string
	exportFormats string
	sharedWithMe  bool
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
	browserOpened bool
	browserError  string
}

func (s *Service) BeginConfig(input ConfigInput) (resultID string, resultErr error) {
	defer func() { s.diagnostics.Operation("setup", resultErr) }()
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return "", err
	}
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
	if input.ImportDraftID != "" {
		found := false
		for _, draft := range s.store.ImportDrafts {
			if draft.ID == input.ImportDraftID {
				found = true
			}
		}
		if !found || c.ID != "" || !input.Guided {
			return "", fmt.Errorf("Imported definition not found.")
		}
	}
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
		if worker := s.workers[old.Remote]; worker != nil && !worker.Alive() && !s.busy[c.ID] {
			s.mounted[c.ID] = false
			s.stopConnection(old)
		}
		if old.SessionOnly != c.SessionOnly && old.Recovery {
			return "", fmt.Errorf("Resolve pending recovery before changing credential storage.")
		}
		if !old.SessionOnly && c.SessionOnly && !input.ForgetCredentials {
			return "", &FieldError{Field: "sessionOnly", Message: "Confirm removal of saved credentials before enabling session-only sign-in."}
		}
		if old.SessionOnly != c.SessionOnly {
			ctx, done := context.WithTimeout(context.Background(), 5*time.Second)
			pending, err := s.cachedPending(ctx, old)
			done()
			if err != nil || pending != 0 {
				return "", fmt.Errorf("unmount and resolve pending files before editing")
			}
		}
		if s.mounted[c.ID] || s.busy[c.ID] {
			return "", fmt.Errorf("unmount before reconfiguring")
		}
		if !old.SessionOnly {
			if err := s.loadConnectionRPC(old); err != nil {
				return "", err
			}
		}
		oldRemote = old.Remote
		c.CardStyle, c.IconImage = old.CardStyle, old.IconImage
		if old.Recovery {
			if showHiddenFiles(c) != showHiddenFiles(old) || c.Provider != old.Provider || c.Path != old.Path || mountTarget(c) != mountTarget(old) || c.ReadOnly != old.ReadOnly || c.DirectoryCacheSeconds != old.DirectoryCacheSeconds || c.VirtualCapacityGiB != old.VirtualCapacityGiB {
				return "", fmt.Errorf("preserve provider, folder, letter and access mode during recovery")
			}
			if c.UploadLimit != old.UploadLimit || c.DownloadLimit != old.DownloadLimit || c.UseProxy != old.UseProxy || c.ConnectTimeout != old.ConnectTimeout || c.IdleTimeout != old.IdleTimeout || c.AlternateURL != old.AlternateURL {
				return "", fmt.Errorf("Preserve network settings during pending recovery.")
			}
			browser, privateBrowser := c.Browser, c.PrivateBrowser
			c = old
			c.Browser, c.PrivateBrowser = browser, privateBrowser
			inPlace = true
		}
	} else {
		c.ID = newID()
	}
	if !input.Guided && (c.SessionOnly || c.AlternateURL != "") {
		return "", fmt.Errorf("Use guided setup to verify the selected destination.")
	}
	if !inPlace {
		c.ResourceID = ""
		c.PrimaryURL = ""
		c.Binding = ""
	}
	for _, existing := range s.store.Connections {
		if existing.ID != c.ID && mountTargetsOverlap(existing, c) {
			field := "letter"
			if c.MountFolder != "" {
				field = "mountFolder"
			}
			return "", &FieldError{Field: field, Message: "The mount destination is reserved by another connection."}
		}
	}
	p, _ := provider(c.Provider)
	if c.AuthMethod == "" {
		if p.OAuth {
			c.AuthMethod = "oauth"
		}
		if p.ID == "gcs" && input.Values["service_account_file"] != "" {
			c.AuthMethod = "serviceAccount"
		}
	}
	if p.ID == "gcs" {
		if c.AuthMethod == "serviceAccount" && input.Values["service_account_file"] == "" {
			return "", fieldMessage("service_account_file", "validation.required", map[string]string{"label": "Service account JSON file"})
		}
		if c.AuthMethod == "oauth" && input.Values["service_account_file"] != "" {
			return "", fmt.Errorf("Select one authentication method before continuing.")
		}
	}
	params, err := parameters(p, input.Values, input.AllowInsecure, input.Transport)
	if err != nil {
		return "", err
	}
	sessionCtx, stop := context.WithCancel(context.Background())
	ctx, cancel := context.WithTimeout(sessionCtx, 10*time.Minute)
	ss := &ConfigSession{ID: newID(), Busy: true, connection: c, ConnectionID: c.ID, cancel: cancel, ctx: sessionCtx, stop: stop, rpc: s.connectionRPC(c), done: make(chan struct{}), inPlace: inPlace, guided: input.Guided}
	s.session = ss
	ss.importDraftID = input.ImportDraftID
	ss.sharedWithMe = params["shared_with_me"] == "true"
	ss.exportFormats = params["export_formats"]
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
	getConfig := func(name string, out *map[string]string) error {
		getCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		defer stop()
		legacy := s.rpc
		s.mu.Unlock()
		err := callRPC(getCtx, ss.rpc, "config/get", map[string]string{"name": name}, out)
		if err == nil && len(*out) == 0 && ss.rpc != legacy && !c.SessionOnly {
			err = callRPC(getCtx, legacy, "config/get", map[string]string{"name": name}, out)
		}
		if c.SessionOnly && c.Binding != "" {
			err = json.Unmarshal([]byte(c.Binding), out)
		}
		s.mu.Lock()
		if s.session != ss || ss.closing || ctx.Err() != nil {
			return fmt.Errorf("setup changed; try again")
		}
		return err
	}
	if p.OAuth && !(p.ID == "gcs" && c.AuthMethod == "serviceAccount") {
		if oldRemote != "" {
			var existing map[string]string
			err := getConfig("sd_"+oldRemote, &existing)
			if err != nil {
				return "", fmt.Errorf("could not load access details; cancel and try again")
			}
			if p.ID == "drive" && !inPlace {
				previous := existing["export_formats"]
				if previous == "" {
					previous = "docx,xlsx,pptx,svg"
				}
				if previous != params["export_formats"] {
					checkCtx, done := context.WithTimeout(ctx, 5*time.Second)
					pending, cacheErr := s.cachedPending(checkCtx, Connection{Remote: oldRemote})
					done()
					if cacheErr != nil || pending != 0 {
						return "", fmt.Errorf("unmount and resolve pending files before editing")
					}
				}
			}
			if p.ID == "drive" && existing["type"] == "drive" && params["shared_with_me"] != "true" && existing["team_drive"] != "" {
				params["team_drive"] = existing["team_drive"]
			}
			if params["client_id"] == "" {
				params["client_id"] = existing["client_id"]
			}
			if params["client_secret"] == "" && params["client_id"] == existing["client_id"] {
				params["client_secret"] = existing["client_secret"]
			}
		}
		if err = oauthParameters(p, params); err != nil {
			return "", err
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
	if s.startConnection != nil {
		params["config_auth_no_browser"] = "true"
	}
	recreate := false
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
			if (key == "disable_epsv" || key == "disable_mlsd" || key == "shared_with_me") && value == "false" && existing[key] == "" {
				continue
			}
			if key == "export_formats" && value == "docx,xlsx,pptx,svg" && existing[key] == "" {
				continue
			}
			if key == "sas_url" {
				next, _ := url.Parse(value)
				old, _ := url.Parse(existing[key])
				if next == nil || old == nil || next.Host != old.Host || next.Path != old.Path {
					return "", fmt.Errorf("the SAS URL must keep the destination during recovery")
				}
			}
			if key == "config_is_local" || key == "config_auth_no_browser" || key == "client_id" || key == "client_secret" || key == "scope" || key == "host_keys" || key == "access_key_id" || key == "service_account_file" || (key == "account" && p.ID == "b2") {
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
		if p.OAuth {
			recreate = true
			for key, value := range destinationFields(c, existing) {
				if key != "type" && key != "profile" && key != "path" {
					if _, ok := params[key]; !ok {
						params[key] = value
					}
				}
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
	delete(s.transportInspections, c.ID)
	if ss.rpc, err = s.prepareConnection(ctx, c, inPlace && !recreate); err != nil {
		return "", err
	}
	ss.connection, ss.oldRemote = c, oldRemote
	id := ss.ID
	method := "config/create"
	if inPlace {
		method = "config/update"
		if (c.SessionOnly || recreate) && s.startConnection != nil {
			method = "config/create"
			var binding map[string]string
			if c.SessionOnly && json.Unmarshal([]byte(c.Binding), &binding) != nil {
				return "", fmt.Errorf("Invalid saved destination binding. Preserve pending files.")
			}
			for key, value := range binding {
				if key != "type" && key != "profile" && key != "path" {
					if _, ok := params[key]; !ok {
						params[key] = value
					}
				}
			}
		}
	}
	started = true
	go func() {
		defer cancel()
		defer close(done)
		s.configure(ctx, id, ss.rpc, method, map[string]any{"name": "sd_" + c.Remote, "type": p.Backend, "parameters": params, "opt": map[string]any{"nonInteractive": true, "obscure": true}})
	}()
	return id, nil
}

func (s *Service) configure(ctx context.Context, id string, rpc RPC, method string, input map[string]any) {
	if s.startConnection != nil {
		go s.launchAuthorization(ctx, id, rpc)
	}
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
		if ss.browserError != "" {
			ss.Error = ss.browserError
		}
		return
	}
	if err != nil {
		ss.Error = setupFailure(err)
		if ss.browserError != "" {
			ss.Error = ss.browserError
		}
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
		if p.ID == "drive" && ss.sharedWithMe != (config["shared_with_me"] == "true") {
			ss.Error = "The engine did not preserve the selected Google Drive contents. Cancel and try again."
			return
		}
		if p.ID == "drive" && config["export_formats"] != ss.exportFormats && !(config["export_formats"] == "" && ss.exportFormats == "docx,xlsx,pptx,svg") {
			ss.Error = "the engine did not save a valid account configuration; cancel and try again"
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
	if ss.importDraftID != "" {
		for _, other := range s.store.Connections {
			if other.ID != ss.connection.ID && strings.EqualFold(strings.TrimSpace(other.Name), strings.TrimSpace(ss.connection.Name)) {
				return "Resolve duplicate names before importing."
			}
		}
	}
	if i := s.index(ss.connection.ID); i >= 0 && !s.store.Connections[i].SessionOnly && ss.connection.SessionOnly && ss.oldRemote != "" {
		if err := s.forgetRemote(ss.oldRemote); err != nil {
			return "Could not remove saved credentials. Session-only mode was not enabled."
		}
		ss.oldRemote = ""
	}
	for _, other := range s.store.Connections {
		if other.ID != ss.connection.ID && mountTargetsOverlap(other, ss.connection) {
			return "The mount destination is reserved by another connection."
		}
	}
	previous := append([]Connection{}, s.store.Connections...)
	previousDrafts := s.store.ImportDrafts
	if ss.importDraftID != "" {
		s.store.ImportDrafts = []ImportDraft{}
		for _, draft := range previousDrafts {
			if draft.ID != ss.importDraftID {
				s.store.ImportDrafts = append(s.store.ImportDrafts, draft)
			}
		}
	}
	if i := s.index(ss.connection.ID); i >= 0 {
		s.store.Connections[i] = ss.connection
	} else {
		s.store.Connections = append(s.store.Connections, ss.connection)
	}
	if err := saveStore(s.dir, s.store); err != nil {
		s.store.Connections = previous
		s.store.ImportDrafts = previousDrafts
		return "Could not save preferences; cancel and try again"
	}
	ss.Complete = true
	delete(s.inspections, ss.connection.ID)
	delete(s.transportInspections, ss.connection.ID)

	if ss.oldRemote != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		name := ss.oldRemote
		ss.Busy = true
		oldRPC := s.connectionRPC(Connection{Remote: name})
		s.mu.Unlock()
		err := callRPC(ctx, oldRPC, "config/delete", map[string]string{"name": "sd_" + name}, nil)
		if err == nil && oldRPC != s.rpc {
			err = callRPC(ctx, s.rpc, "config/delete", map[string]string{"name": "sd_" + name}, nil)
		}
		s.mu.Lock()
		ss.Busy = false
		if err != nil {
			return "Connection saved; old credential cleanup failed. Try Done again."
		}
		s.stopConnection(Connection{Remote: name})
		ss.oldRemote = ""
	}

	return ""
}

func (s *Service) ConfigStatus(id string) (ConfigSession, error) {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return ConfigSession{}, err
	}
	defer s.mu.Unlock()
	if s.session == nil || s.session.ID != id {
		return ConfigSession{}, fmt.Errorf("setup not found")
	}
	return *s.session, nil
}
func (s *Service) AnswerConfig(id, answer string) error {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return err
	}
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
func (s *Service) EndConfig(id string) error {
	if err := s.accessAllowed(); err != nil {
		return err
	}
	if id == "" {
		s.mu.Lock()
		if s.session != nil {
			id = s.session.ID
		}
		s.mu.Unlock()
	}
	err := s.endConfig(id)
	if err == nil {
		s.mu.Lock()
		if strings.HasPrefix(s.access.warning, "Setup cancellation") {
			s.access.warning = ""
			s.accessChanged()
		}
		s.mu.Unlock()
	}
	return err
}

func (s *Service) endConfig(id string) (resultErr error) {
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
	cleanupRPC, legacy := rpc, s.rpc
	if ss.Complete && name != "" {
		cleanupRPC = s.connectionRPC(Connection{Remote: name})
	}
	s.mu.Unlock()
	var cleanupErr error
	if cleanupRPC != nil && cleanupRPC.Alive() && name != "" {
		cleanupErr = cleanupRPC.Call(ctx, "config/delete", map[string]string{"name": "sd_" + name}, nil)
	}
	if cleanupErr == nil && ss.Complete && name != "" && cleanupRPC != legacy {
		cleanupErr = callRPC(ctx, legacy, "config/delete", map[string]string{"name": "sd_" + name}, nil)
	}
	s.mu.Lock()
	if cleanupErr != nil {
		return cleanupErr
	}
	if s.session != ss {
		return nil
	}
	if ss.Complete && name != "" {
		s.stopConnection(Connection{Remote: name})
	}
	if !ss.inPlace && !ss.Complete || ss.connection.SessionOnly && !s.mounted[ss.connection.ID] {
		s.stopConnection(ss.connection)
	}
	s.session = nil
	return nil
}

type HostIdentity struct {
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint"`
}

func (s *Service) ProbeSSH(host, port string) (HostIdentity, error) {
	if err := s.accessAllowed(); err != nil {
		return HostIdentity{}, err
	}
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

type ConnectionFieldsView struct {
	Values    map[string]string `json:"values"`
	Transport string            `json:"transport"`
}

func (s *Service) ConnectionFields(id string) (ConnectionFieldsView, error) {
	s.mu.Lock()
	if err := s.requireUnlocked(); err != nil {
		s.mu.Unlock()
		return ConnectionFieldsView{}, err
	}
	i := s.index(id)
	if i < 0 {
		s.mu.Unlock()
		return ConnectionFieldsView{}, fmt.Errorf("connection not found")
	}
	epoch := s.access.epoch
	c := s.store.Connections[i]
	if !c.SessionOnly {
		if err := s.loadConnectionRPC(c); err != nil {
			s.mu.Unlock()
			return ConnectionFieldsView{}, err
		}
	}
	rpc, legacy := s.connectionRPC(c), s.rpc
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var config map[string]string
	if c.SessionOnly && c.Binding != "" {
		_ = json.Unmarshal([]byte(c.Binding), &config)
	}
	if config == nil {
		if err := callRPC(ctx, rpc, "config/get", map[string]string{"name": "sd_" + c.Remote}, &config); err != nil {
			return ConnectionFieldsView{}, fmt.Errorf("could not load access details; cancel and try again")
		}
	}
	if len(config) == 0 && rpc != legacy && !c.SessionOnly {
		if err := callRPC(ctx, legacy, "config/get", map[string]string{"name": "sd_" + c.Remote}, &config); err != nil {
			return ConnectionFieldsView{}, fmt.Errorf("could not load access details; cancel and try again")
		}
	}
	s.mu.Lock()
	current := s.index(id)
	valid := !s.access.locked && s.access.epoch == epoch && s.connectionRPC(c) == rpc && current >= 0 && s.store.Connections[current] == c
	s.mu.Unlock()
	if !valid {
		return ConnectionFieldsView{}, fmt.Errorf("connection changed; try again")
	}
	p, _ := provider(c.Provider)
	if config == nil || config["type"] != p.Backend || validateDriveContents(config) != nil {
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
