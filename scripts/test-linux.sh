#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.."
[[ $(uname -s) == Linux ]] || { echo 'Linux is required.' >&2; exit 1; }
export GOTOOLCHAIN=go1.26.10
export LIBGL_ALWAYS_SOFTWARE=1
[[ -r /dev/fuse && -w /dev/fuse ]] || { echo '/dev/fuse is required for native checks.' >&2; exit 1; }
command -v fusermount3 >/dev/null
command -v dbus-run-session >/dev/null
command -v gnome-keyring-daemon >/dev/null
command -v Xvfb >/dev/null
command -v weston >/dev/null
(cd frontend && npm test)
go vet ./...
go test ./internal/i18n ./internal/diagnostics ./internal/platform ./internal/engine -count=1
# Windows-specific fixture suites remain covered on Windows; Linux has dedicated lifecycle tests.
go test ./internal/core -run 'TestLinux|TestBridgeContracts|TestCacheInspection|TestCacheMetadata|TestCriticalCacheStatsValidation|TestAboutUsesRunningEngine' -count=1
SDRIVE_TEST_ENGINE=1 go test ./internal/engine -count=1
SDRIVE_TEST_MOUNT=1 go test ./tests/integration -run '^TestLinux' -count=1 -timeout 3m
SDRIVE_TEST_UI=1 GDK_BACKEND=x11 xvfb-run -a go test ./internal/desktop -run '^TestLinuxNativeDesktop$' -count=1 -timeout 90s
runtime=$(mktemp -d)
trap 'kill "$weston_pid" 2>/dev/null || true; rmdir "$runtime" 2>/dev/null || true' EXIT
chmod 700 "$runtime"
export XDG_RUNTIME_DIR="$runtime" WAYLAND_DISPLAY=sdrive-test
weston --backend=headless --socket="$WAYLAND_DISPLAY" --idle-time=0 > "$runtime/weston.log" 2>&1 &
weston_pid=$!
for attempt in {1..50}; do [[ -S "$runtime/$WAYLAND_DISPLAY" ]] && break; sleep 0.1; done
[[ -S "$runtime/$WAYLAND_DISPLAY" ]]
SDRIVE_TEST_UI=1 GDK_BACKEND=wayland go test ./internal/desktop -run '^TestLinuxNativeDesktop$' -count=1 -timeout 90s
