package platform

import "testing"

func TestBrowserDiscoveryExposesOnlySupportedLaunchers(t *testing.T) {
	browsers := InstalledBrowsers()
	if len(browsers) == 0 || browsers[0].ID != "" {
		t.Fatal("missing default browser")
	}
	seen := map[string]bool{}
	for _, browser := range browsers {
		if seen[browser.ID] || browser.Name == "" {
			t.Fatal("invalid browser discovery")
		}
		seen[browser.ID] = true
		switch browser.ID {
		case "", "edge", "chrome", "firefox":
		default:
			t.Fatal("unsupported browser exposed")
		}
	}
	if err := OpenSignIn("cmd.exe", true, "http://127.0.0.1:53682/auth?state=synthetic"); err == nil {
		t.Fatal("arbitrary executable accepted as a browser")
	}
}
