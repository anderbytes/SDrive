# SDrive development

This guide is for developers and maintainers. End users only need the installer. Repository documentation and source messages are written in English; see [localization](localization.md) for interface translations.

## Build and verify

Development prerequisite: Node.js 24 LTS or later and npm. In PowerShell, from the repository root:

```powershell
./scripts/build.ps1 -Check -Installer
```

The script downloads **Go 1.26.10** and, for the installer, **NSIS 3.12** into `.tools/`, verifying SHA-256. **rclone 1.75.2 is built from official sources**, with a pinned module checksum, Go 1.26.10 and the `cmount` tag, without a C/C++ compiler. This avoids distributing the official binary built with Go 1.26.9, before the GO-2026-6809 fix. The global PATH is unchanged. Wails is pinned to **v3.0.0-beta.34**, alongside its matching JavaScript runtime. Libraries use `go.sum` and `package-lock.json`.

Outputs in `bin/`: application, rclone, per-user online and offline installers, SDrive and rclone sources, licenses and `SHA256SUMS`. Both installers include sources and licenses. Local executables have no publisher digital signature.

Individual checks:

```powershell
./scripts/bootstrap.ps1
./.tools/go/bin/go.exe test ./...
./.tools/go/bin/go.exe vet ./...
npm --prefix frontend test
npm --prefix frontend run build
./bin/SDrive-check.exe --self-check
node scripts/ui-smoke.mjs
```

The smoke test opens a hidden WebView2 using an isolated profile in `.tools/ui-check/`. It checks all 18 options, creates a temporary connection to a local test WebDAV server, browses folders, cancels without persisting the connection, saves a test preference and exits. It does not use personal accounts or mount drives. Debugging port 9337 exists only in that test process. Do not run it while another SDrive instance is open.

Settings and key regressions can be run without mounting drives or changing the Windows startup registry:

```powershell
./.tools/go/bin/go.exe test ./internal/core ./internal/platform ./internal/i18n
npm --prefix frontend test
npm --prefix frontend run build
```

The settings tests use temporary profiles, injected startup operations and temporary file locks to exercise failed replacements. Platform tests exercise real DPAPI under the current Windows user and exclusive atomic key creation, including concurrent callers. Frontend tests cover restoration confirmation/cancellation, failure feedback, protected recovery and maintenance retries in jsdom. These checks do not prove native WebView interaction, abrupt power-loss durability, real mounted-cache recovery or installer behavior. See [local settings recovery](security.md#local-settings-recovery) for the persisted format, backup policy and manual recovery limits.

## Concurrency and real-engine checks

After bootstrap, run the Windows race detector with:

```powershell
./scripts/test-race.ps1
```

This uses Go 1.26.10 and SHA-256-verified w64devkit 2.10.1 (GCC 16.2.0) in `.tools/`, sets `CGO_ENABLED=1` and `CC` for the command, and restores their previous values. It does not modify PATH or install a system compiler. The Windows workflow runs the same script with the disposable real-engine checks enabled.

```powershell
$env:SDRIVE_TEST_ENGINE = '1'
try { ./scripts/test-race.ps1 }
finally { Remove-Item Env:SDRIVE_TEST_ENGINE }
```

The opt-in engine test uses a temporary DPAPI-protected profile and the pinned local rclone binary. It verifies asynchronous configuration, cancellation after an accepted write, terminal job status and remote deletion. It needs no provider account, browser login or drive mount. HTTP tests additionally simulate delayed writes, lost start responses and unknown completion. Core concurrency tests use channels for folder cancellation, late confirmation, snapshot/restart, overlapping snapshots, duplicate mounts, removal during mounting and exit during mounting. Mount tests inject only the WinFsp availability check and use RPC doubles; they do not install WinFsp or mount native drives.

Ownership tests use separate processes and a hard-link alias of the lock file, check the installer's native sharing-violation probe, preserve the legacy mutex marker and verify release after process termination. They do not create an interactive second Windows session. Native WinFsp mounting remains covered by the separate opt-in `SDRIVE_TEST_MOUNT=1` lifecycle test.

## Connection lifecycle checks

`ConfirmConfig(sessionID, path, letter)` validates the chosen Windows letter before saving. Repeating confirmation reuses the saved connection; changing its letter requires a disconnected, idle connection with a verified clean cache and no recovery protection. Other saved fields and the remote identity remain unchanged. Connection snapshots retain `busy` and add transient `operation`: empty when idle, otherwise `validating`, `mounting`, `checkingUploads`, `waitingUploads` or `unmounting`. These stages are not persisted. An access test only lists folders; success does not prove write permission.

Core regressions inject drive-letter availability and RPC responses, including failures before mounting, occupied-letter changes during destination validation, saved-connection retries, persistence rollback and channel-controlled operation stages. Frontend tests cover unavailable letters, saved configuration feedback, Google Drive consent restrictions and English/Portuguese messages. Run the core/i18n tests, frontend tests/build and the race detector described above; none requires a provider account or native drive mounting.

### Suspend and connectivity recovery (manual checks)

Use a disposable Windows VM with WinFsp, an explicitly selected test account/folder and disposable files. Record the build, provider/account type and sanitized results for each scenario. Save expected file hashes before starting; keep credentials and cache intact when a scenario fails. Do not change the workstation's power or network settings to perform these checks.

- Establish read/list access and, on a writable connection, create a test file and verify its downloaded hash. Repeat read/list checks on a read-only connection without claiming write capability.
- While a disposable write is queued, suspend and resume; repeat with hibernation if supported. Check the actual mount, upload status and destination after each resume. Record unsupported VM power states as untested.
- With pending writes, disconnect/reconnect the VM network, change Wi-Fi/network adapters where supported, and connect/disconnect its test VPN. Verify errors and unknown status remain visible and pending files stay recoverable.
- Expire or revoke only the selected test credentials. Verify access failure is actionable, then renew against the original account/library and recover. Do not redirect pending uploads to a different destination.
- After each scenario, use Connect/Recover as needed, wait for uploads, disconnect safely and verify the remote file hash. Confirm there is one saved connection, the letter/destination remain correct and pending data was neither lost nor reported complete while unknown. Remove only the disposable remote files after verification.

These are manual acceptance scenarios, not completed validation. Report native mount availability, recovery after resume and any intervention needed separately; local tests do not certify automatic reconnection or live providers.

## Sign-in identities

Authentication regressions also verify strict completion response fields against the pinned local rclone binary, all three FTP transports and password obscuring in a temporary encrypted profile. The FTP configuration checks do not contact a storage server. Setup verifies saved remotes before success; frontend tests cover failed access-detail loading, renewed transport/consent, sensitive-question masking, show/hide controls and service-account availability. `ConnectionFields` returns `{values, transport}` to the frontend; `values` includes only non-secret catalog fields. See the [provider identity limits](provider-validation.md#recovery-identity-limits) before describing these checks as account verification.

`internal/core/oauth.json` is embedded in the application and starts empty. Registration is a maintainer task: register the desktop application with the provider, configure rclone-compatible local redirects, complete required consent/approvals and test sign-in. This is not an end-user installation step.

The format is an object per profile ID containing `clientId` and, only when required for a public desktop client, `clientSecret`. OAuth profiles: `drive`, `onedrive`, `sharepoint`, `dropbox`, `box`, `pcloud` and `gcs`. OneDrive and SharePoint can share a registration, with an explicit entry for each profile. GCS also accepts a service account file without an OAuth identity.

These values will be public in the executable and distributed source archive. Never put server secrets or personal account credentials in this file. No user tokens belong in a build. The flow remains between the application, browser and provider, without an SDrive intermediary server.

Without a configured identity, new OAuth sign-in is unavailable with a clear message, while the provider stays in the catalog. Existing connections with a custom identity keep it during renewal if the project has not configured one yet. Public integration release also requires live validation; filling in the file does not prove provider approval. Google Drive requires attention to [shared client deprecation](https://rclone.org/drive/#making-your-own-client-id) and [verification requirements](https://developers.google.com/identity/protocols/oauth2/production-readiness/restricted-scope-verification).

## Online and offline installers

`installer/prerequisites.json` is the single source for addresses, SHA-256 values and publishers of official WinFsp and WebView2 x64 installers. The manifest is embedded in the application. `scripts/prerequisites.ps1`, called during packaging, downloads files for the offline package, checks hashes and signatures and stops packaging on mismatch. Packaging also runs the native Windows signature test against both official files. It does not automatically switch to newer versions: when updating the manifest, validate origin, signature and compatibility.

`scripts/package.ps1` compiles the same NSIS script twice: without an additional definition for `SDrive-0.1.0-setup.exe` (online), and with `/DOFFLINE` for `SDrive-0.1.0-offline-setup.exe`. CI delivers both and their checksums. SDrive, rclone, sources and licenses are included in both.

The online installer first checks installed components and calls the Go helper to download only those needed, using the same pinned official files as the offline variant. Users do not need PowerShell, download plugins or scripts. The helper accepts only `winfsp` and `webview2` IDs; URLs are not accepted through the command line or Wails bridge.

Downloads use HTTPS, reject HTTP redirects, have a 512 MiB per-file limit, a 30-minute total timeout and cancellation after 60 seconds without receiving data. Partial files are removed on error; there is no permanent cache. English progress is sent as UTF-16 to the native NSIS log. Errors offer retry or cancellation. An already verified file in the temporary directory is reused during the same installation.

Both variants verify SHA-256, Authenticode signature and publisher CN through native Windows APIs before executing each component, and keep the file locked against writes and deletion until execution finishes. Trust verification uses local certificates and does not require online revocation queries, preserving offline installation.

The package runs without elevation, preserves the original user and elevates only the WinFsp MSI. Exit codes 0 and 3010 mean success and success with restart pending. Authorization cancellation or installation failure prevents reporting completion. WebView2 is installed using `/silent /install`, following [Microsoft documentation](https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution#offline-deployment).

## Installer acceptance on a clean machine

Run these scenarios in a disposable Windows 11 x64 VM; do not remove shared components from your workstation to simulate their absence:

- Install offline without WinFsp/WebView2 and without internet access; open the wizard after completion or restart when requested.
- Install online with both absent; watch progress and verify both dependencies are prepared automatically.
- Install online with both present and no internet; no download should occur.
- Interrupt the network during download, retry and cancel; no partial file may execute and the existing application must be preserved.
- Repeat with both present and with only one present; confirm compatible components are reused.
- Use a standard user and authorize with another administrator; data and credentials must stay in the original user's profile.
- Cancel authorization, simulate MSI failure and test exit code 3010; no automatic restart or false success.
- Run the installer again for repair, update while preserving connections/cache, and uninstall while preserving data/shared components.
- Confirm users do not need to run terminals or commands.

These scenarios require the VM and UAC interaction. Compiling NSIS and running mocked tests do not replace this validation.
