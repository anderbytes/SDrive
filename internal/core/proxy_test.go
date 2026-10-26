package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"sdrive/internal/testutil"
)

func TestSOCKSProxyValidationAndCredentialPersistence(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			s := testService(t, nil)
			p := ProxyConfig{Address: scheme + "://127.0.0.1:1080", Username: "synthetic-user", Password: "synthetic-password"}
			if err := s.SaveProxy(p); err != nil {
				t.Fatal(err)
			}
			view, err := s.ProxySettings()
			if err != nil || view.Password != "" || !view.HasPassword || view.Address != p.Address {
				t.Fatal("proxy settings did not reopen without exposing the password")
			}
			if err := s.SaveProxy(view); err != nil {
				t.Fatal(err)
			}
			saved, err := s.readProxy()
			if err != nil || saved.Password != p.Password {
				t.Fatal("saved SOCKS password lost")
			}
			for _, change := range []func(*ProxyConfig){
				func(p *ProxyConfig) { p.Username = "" },
				func(p *ProxyConfig) { p.Password = "" },
				func(p *ProxyConfig) { p.Username = strings.Repeat("é", 128) },
				func(p *ProxyConfig) { p.Password = strings.Repeat("x", 256) },
				func(p *ProxyConfig) { p.Address = scheme + "://user:secret@localhost:1080" },
				func(p *ProxyConfig) { p.Address += "/path" },
				func(p *ProxyConfig) { p.Address += "?query=secret" },
				func(p *ProxyConfig) { p.Address = scheme + "://:1080" },
				func(p *ProxyConfig) { p.Address = "socks4://localhost:1080" },
			} {
				invalid := p
				change(&invalid)
				if err := s.SaveProxy(invalid); err == nil {
					t.Fatal("invalid SOCKS settings accepted")
				}
			}
			for _, valid := range []ProxyConfig{
				{Address: p.Address},
				{Address: p.Address, Username: strings.Repeat("x", 255), Password: strings.Repeat("y", 255)},
			} {
				if err := s.SaveProxy(valid); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestSOCKSProxyDestinationVerification(t *testing.T) {
	for _, scheme := range []string{"socks5", "socks5h"} {
		t.Run(scheme, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != "PROPFIND" || r.URL.Path != "/dav/" {
					t.Error("unexpected identity request")
				}
				w.WriteHeader(207)
				fmt.Fprint(w, identityResponse("/dav", "urn:uuid:resource"))
			}))
			defer server.Close()
			proxy := testutil.NewSOCKSProxy(t, server.Listener.Addr().String(), "synthetic-user", "synthetic-password")
			s := testService(t, nil)
			p := ProxyConfig{Address: strings.Replace(proxy.URL, "socks5:", scheme+":", 1), Username: "synthetic-user", Password: "synthetic-password"}
			if err := s.SaveProxy(p); err != nil {
				t.Fatal(err)
			}
			client, err := s.destinationClient(Connection{UseProxy: true})
			if err != nil {
				t.Fatal(err)
			}
			defer client.CloseIdleConnections()
			client.Transport.(*http.Transport).TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			// The fixture certificate includes this address; only its routing changes.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			id, _, err := probeResource(ctx, client, server.URL+"/dav", "", "", "")
			if err != nil || id != resourceDigest("urn:uuid:resource") || proxy.Requests.Load() != 1 {
				t.Fatalf("identity did not use SOCKS proxy: %v", err)
			}
			client.CloseIdleConnections()
			proxy.Close()
			if _, _, err := probeResource(ctx, client, server.URL+"/dav", "", "", ""); err == nil || requests.Load() != 1 {
				t.Fatal("failed SOCKS proxy bypassed for destination verification")
			}
		})
	}
}
