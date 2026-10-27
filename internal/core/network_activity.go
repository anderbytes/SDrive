package core

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

// NetworkView keeps socket bytes separate from application file contents.
type NetworkView struct {
	State         string     `json:"state"`
	Partial       bool       `json:"partial"`
	StartedAt     *time.Time `json:"startedAt"`
	CheckedAt     *time.Time `json:"checkedAt"`
	Sent          *int64     `json:"sent"`
	Received      *int64     `json:"received"`
	FilesSent     *int64     `json:"filesSent"`
	FilesReceived *int64     `json:"filesReceived"`
}

type networkSession struct {
	id                       string
	rpc                      RPC
	remote                   string
	stopped                  bool
	partial                  bool
	view                     NetworkView
	probeSent, probeReceived int64
}

type networkProbeConn struct {
	net.Conn
	s       *Service
	id      string
	session *networkSession
}

func (c *networkProbeConn) record(n int, sent bool) {
	if n <= 0 {
		return
	}
	c.s.mu.Lock()
	defer c.s.mu.Unlock()
	if c.s.networkSessions[c.id] != c.session || c.session.stopped {
		return
	}
	if sent {
		c.session.probeSent += int64(n)
	} else {
		c.session.probeReceived += int64(n)
	}
}

func (c *networkProbeConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.record(n, false)
	return n, err
}

func (c *networkProbeConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.record(n, true)
	return n, err
}

func mergeNetworkProbes(view NetworkView, session *networkSession) NetworkView {
	if view.Sent != nil {
		sent, received := *view.Sent+session.probeSent, *view.Received+session.probeReceived
		view.Sent, view.Received = &sent, &received
	}
	return view
}

func networkProgress(previous, next NetworkView) bool {
	return previous.CheckedAt == nil || next.StartedAt.Equal(*previous.StartedAt) && *next.Sent >= *previous.Sent && *next.Received >= *previous.Received && *next.FilesSent >= *previous.FilesSent && *next.FilesReceived >= *previous.FilesReceived
}

func readNetwork(ctx context.Context, session *networkSession, action string) (NetworkView, error) {
	var response struct {
		Session                                  string
		Active                                   *bool
		StartedAt, CheckedAt                     *time.Time
		Sent, Received, FilesSent, FilesReceived *int64
	}
	err := callRPC(ctx, session.rpc, "sdrive/network", map[string]string{"action": action, "session": session.id}, &response)
	if err != nil {
		return NetworkView{}, err
	}
	if response.Session != session.id || response.Active == nil || response.StartedAt == nil || response.CheckedAt == nil || response.StartedAt.IsZero() || response.CheckedAt.Before(*response.StartedAt) {
		return NetworkView{}, fmt.Errorf("invalid network session")
	}
	for _, n := range []*int64{response.Sent, response.Received, response.FilesSent, response.FilesReceived} {
		if n == nil || *n < 0 {
			return NetworkView{}, fmt.Errorf("invalid network counters")
		}
	}
	if action == "stop" && *response.Active || action != "stop" && !*response.Active {
		return NetworkView{}, fmt.Errorf("network session is inactive")
	}
	state := "active"
	if session.partial {
		state = "partial"
	}
	if action == "stop" {
		state = "disconnected"
	}
	return NetworkView{State: state, Partial: session.partial, StartedAt: response.StartedAt, CheckedAt: response.CheckedAt, Sent: response.Sent, Received: response.Received, FilesSent: response.FilesSent, FilesReceived: response.FilesReceived}, nil
}

// Called before destination validation; collection failure never blocks a mount.
func (s *Service) startNetwork(ctx context.Context, c Connection, rpc RPC) {
	s.mu.Lock()
	session := &networkSession{id: newID(), rpc: rpc, remote: c.Remote, view: NetworkView{State: "unavailable"}}
	if s.networkSessions == nil {
		s.networkSessions = map[string]*networkSession{}
	}
	s.networkSessions[c.ID] = session
	s.mu.Unlock()
	check, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	view, err := readNetwork(check, session, "start")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.networkSessions[c.ID] == session && s.connectionRPC(c) == rpc && err == nil {
		session.view = view
	}
}

func (s *Service) setNetworkCoverage(c Connection, config map[string]string) {
	session := s.networkSessions[c.ID]
	if session == nil {
		return
	}
	if c.Provider == "sftp" && config["ssh"] != "" {
		session.partial = true
	}
	for key := range config {
		if strings.HasPrefix(key, "override.") {
			session.partial = true
		}
	}
	session.view.Partial = session.partial
	if session.partial && session.view.State == "active" {
		session.view.State = "partial"
	}
}

func (s *Service) sampleNetwork(ctx context.Context, c Connection, rpc RPC) NetworkView {
	s.mu.Lock()
	session := s.networkSessions[c.ID]
	if session == nil || session.remote != c.Remote {
		s.mu.Unlock()
		return NetworkView{}
	}
	previous := session.view
	if session.stopped {
		s.mu.Unlock()
		return previous
	}
	if session.rpc != rpc {
		session.view.State = "stale"
		previous = session.view
		s.mu.Unlock()
		return previous
	}
	copy := *session
	s.mu.Unlock()
	check, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	view, err := readNetwork(check, &copy, "stats")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.networkSessions[c.ID] != session || s.connectionRPC(c) != rpc {
		return NetworkView{}
	}
	if session.stopped {
		return session.view
	}
	if err != nil || !rpc.Alive() {
		if session.view.CheckedAt != nil {
			session.view.State = "stale"
		} else {
			session.view.State = "unavailable"
		}
		return session.view
	}
	view = mergeNetworkProbes(view, session)
	// A slower snapshot must not replace a newer sample or accept a counter reset.
	if !networkProgress(previous, view) {
		session.view.State = "stale"
		return session.view
	}
	if session.view.CheckedAt == nil || !view.CheckedAt.Before(*session.view.CheckedAt) {
		session.view = view
	}
	return session.view
}

// Caller holds s.mu; disconnect owns this session until stopConnection completes.
func (s *Service) stopNetwork(c Connection) {
	session := s.networkSessions[c.ID]
	if session == nil || session.stopped {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	view, err := readNetwork(ctx, session, "stop")
	if err == nil {
		view = mergeNetworkProbes(view, session)
	}
	if err == nil && networkProgress(session.view, view) {
		session.view = view
	} else if session.view.CheckedAt != nil {
		session.view.State = "stale"
	}
	session.stopped = true
}
