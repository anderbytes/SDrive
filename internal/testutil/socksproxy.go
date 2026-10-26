// Package testutil provides disposable loopback fixtures for integration tests.
package testutil

import (
	"bytes"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// SOCKSProxy forwards only storage.invalid or loopback addresses to one local server.
type SOCKSProxy struct {
	URL      string
	Requests atomic.Int64
	Domains  atomic.Int64
	listener net.Listener
	wg       sync.WaitGroup
}

func NewSOCKSProxy(t *testing.T, target, username, password string) *SOCKSProxy {
	t.Helper()
	host, _, err := net.SplitHostPort(target)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		t.Fatal("SOCKS fixture requires a loopback target")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &SOCKSProxy{URL: "socks5://" + listener.Addr().String(), listener: listener}
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			p.wg.Add(1)
			go func() {
				defer p.wg.Done()
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(15 * time.Second))
				p.serve(conn, target, username, password)
			}()
		}
	}()
	t.Cleanup(func() { p.Close(); p.wg.Wait() })
	return p
}

func (p *SOCKSProxy) Close() { p.listener.Close() }

func (p *SOCKSProxy) serve(conn net.Conn, target, username, password string) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:2]); err != nil || header[0] != 5 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(conn, methods); err != nil {
		return
	}
	method := byte(0)
	if username != "" {
		method = 2
	}
	if !bytes.Contains(methods, []byte{method}) {
		conn.Write([]byte{5, 255})
		return
	}
	if _, err := conn.Write([]byte{5, method}); err != nil {
		return
	}
	if method == 2 {
		if _, err := io.ReadFull(conn, header[:2]); err != nil || header[0] != 1 {
			return
		}
		user := make([]byte, int(header[1]))
		if _, err := io.ReadFull(conn, user); err != nil {
			return
		}
		if _, err := io.ReadFull(conn, header[:1]); err != nil {
			return
		}
		secret := make([]byte, int(header[0]))
		if _, err := io.ReadFull(conn, secret); err != nil {
			return
		}
		if string(user) != username || string(secret) != password {
			conn.Write([]byte{1, 1})
			return
		}
		if _, err := conn.Write([]byte{1, 0}); err != nil {
			return
		}
	}
	if _, err := io.ReadFull(conn, header[:]); err != nil || header[0] != 5 || header[1] != 1 || header[2] != 0 {
		return
	}
	var host string
	switch header[3] {
	case 1, 4:
		size := 4
		if header[3] == 4 {
			size = 16
		}
		addr := make([]byte, size)
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
		host = net.IP(addr).String()
	case 3:
		if _, err := io.ReadFull(conn, header[:1]); err != nil {
			return
		}
		addr := make([]byte, int(header[0]))
		if _, err := io.ReadFull(conn, addr); err != nil {
			return
		}
		host = string(addr)
	default:
		return
	}
	if _, err := io.ReadFull(conn, header[:2]); err != nil {
		return
	}
	if host != "storage.invalid" && (net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback()) {
		return
	}
	upstream, err := net.DialTimeout("tcp", target, time.Second)
	if err != nil {
		return
	}
	defer upstream.Close()
	upstream.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err := conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0}); err != nil {
		return
	}
	p.Requests.Add(1)
	if host == "storage.invalid" {
		p.Domains.Add(1)
	}
	done := make(chan struct{})
	go func() { io.Copy(upstream, conn); upstream.(*net.TCPConn).CloseWrite(); close(done) }()
	io.Copy(conn, upstream)
	conn.Close()
	<-done
}
