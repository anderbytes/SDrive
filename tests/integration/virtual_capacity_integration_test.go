//go:build windows

package integration

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsVirtualCapacityAfterReconnect(t *testing.T) {
	requireNativeMount(t)
	f := newIntegrationFixture(t, true)
	s, _ := openIntegrationService(t, f.profile, nil)
	c := configureIntegration(t, s, f.url, f.letter)
	isolatedConnectionWorkers(t, s, f.profile)
	for _, capacity := range []int64{256, 512, 0} {
		c.VirtualCapacityGiB = capacity
		if err := s.UpdateConnection(c); err != nil {
			t.Fatal(err)
		}
		if err := s.Mount(c.ID); err != nil {
			t.Fatal(err)
		}
		point, err := windows.UTF16PtrFromString(c.Letter + `:\`)
		if err != nil {
			t.Fatal(err)
		}
		var available, total, free uint64
		if err := windows.GetDiskFreeSpaceEx(point, &available, &total, &free); err != nil {
			t.Fatal(err)
		}
		if capacity > 0 && total != uint64(capacity<<30) {
			t.Fatalf("Windows total = %d; wanted %d GiB", total, capacity)
		}
		if capacity == 0 && total == 512<<30 {
			t.Fatal("reset retained the virtual total")
		}
		if err := s.Unmount(c.ID); err != nil {
			t.Fatal(err)
		}
	}
}
