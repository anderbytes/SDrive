package platform

import (
	"context"
	"testing"
	"time"
)

func TestFileAccessUnavailableNeverReportsAnUnusedFile(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, path := range []string{"invalid\x00file", `Z:\not-a-real-disposable-resource`} {
		result := FileProcesses(ctx, path)
		if result.State != "unknown" || len(result.Processes) != 0 {
			t.Fatal("unavailable information became a clean result")
		}
	}
	// Restart Manager rejects directory resources with access denied, without changing any ACL.
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if result := FileProcesses(ctx, t.TempDir()); result.State != "unknown" {
		t.Fatal("rejected native resource became a clean result")
	}
}

func TestAccessRecordProtectionRoundTripAndTamperRejection(t *testing.T) {
	sealed, err := ProtectData([]byte("synthetic record"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := UnprotectData(sealed)
	if err != nil || string(plain) != "synthetic record" {
		t.Fatal("protected record did not round trip")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err = UnprotectData(sealed); err == nil {
		t.Fatal("tampered record accepted")
	}
}
