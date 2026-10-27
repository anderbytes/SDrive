# SDrive development

For Linux dependencies, packaging and native X11/Wayland checks, see the [Linux guide](linux.md#build-and-validation).

This guide is for developers and maintainers. End users only need the installer. Repository documentation and source messages are written in English; see [localization](localization.md) for interface translations.

Portable configuration and service icons are described in [portability and desktop preferences](portability-and-desktop.md), including focused validation boundaries.

Application access, inactivity, process discovery and measured Windows Shell operations are described in [access and file operations](access-and-file-operations.md), including focused native test names and acceptance limits.

## Repository structure

The root `main.go` embeds the frontend, prerequisite manifest and package metadata, then delegates to `internal/desktop`. The Windows resource remains beside that entry point. `internal/desktop` owns Wails startup, auxiliary commands, windows, tray menus and disconnect progress; its unit and native UI tests stay in that package. Existing domain logic, rclone integration and Windows APIs remain in `internal/core`, `internal/engine` and `internal/platform`.

Cross-layer tests and their disposable fixtures live in `tests/integration`. Both relocated test packages prepare the repository working directory in `TestMain`, so engine discovery, fixture paths and child processes continue to use the root `.tools/` directory. They retain the `--config-key` subprocess entry required by encrypted rclone configuration. Source archives include both `internal/` and `tests/`, including files not yet tracked by Git.

## Interface layout and visual checks

The shared `DialogLayout` retains native modal semantics through `useDialog`: right-side panels have rounded left corners, space above and below, a fixed heading/footer and a scrollable body. Preferences, drive editing and drive details use vertical section navigation; setup shows its existing Service, Access, Folder and Connect stages. Below 640 px the navigation collapses behind the current-section button. Short confirmations remain centered. Form buttons in fixed footers use explicit form IDs, preserving submission and native field validation across tabs. Busy setup cancellation and one-time recovery acknowledgements retain their original guards.

Drive cards place the name, remote destination and state to the right of the drive letter. Their minimum height is 88 px; long paths and warnings can expand the card. Primary actions stay explicit, while the **…** menu opens the seven-topic details panel and secondary operations. State stripes always accompany text. Themes share geometry, with 44 px action targets and fields. Related field actions share a horizontal row, including file/directory pickers and direct destination navigation; narrow rows wrap button text while retaining input space. Advanced setup settings occupy a separate collapsed block with titled groups and no nested disclosures; unsupported groups are omitted. `FieldHelp` exposes auxiliary explanations on hover, focus or click; Escape dismisses help before dismissing its dialog. Active security consequences and recovery restrictions remain visible. Native and backend validation reveal the relevant section before focusing an invalid field.

Drawers enter in 200 ms; menus, confirmations, new drive cards, revealed content and tabs use 160 ms entry animations. Existing cards retain their identity during status refreshes. Closing remains immediate, and the Windows reduced-motion preference disables animations, including modal backdrops. Animations do not delay focus or actions.

Run `node scripts/ui-visual-smoke.mjs` for fixture-based rendering of the actual React components at 1440 × 900, 1180 × 800, 850 × 620 and 420 × 740. It checks overflow, panel footers, seven details topics, setup, advanced groups and validation, horizontal field actions, animation entry/reopening and reduced motion, preference sections, contextual help, nested import panels, access settings, diagnostics, all supported languages, dark theme and forced colors. Screenshots and the result record are written under `.tools/check-results/desktop-serene/`. The test uses an existing Edge installation, or the executable supplied through the process-scoped `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH` variable. Its Vite fixture replaces only the test server's bridge module; application code and personal profiles are unchanged. These visual fixtures do not validate provider access or native mounting.

Use `node scripts/ui-smoke.mjs` separately for the real Wails/WebView2 handoff with an isolated profile and local WebDAV fixture. Reader software, physical DPI and live providers require separate evidence.

## Build and verify

Development prerequisite: Node.js 24 LTS or later and npm. In PowerShell, from the repository root:

```powershell
./scripts/build.ps1 -Check -Installer
```

The script downloads **Go 1.26.10** and, for the installer, **NSIS 3.12** into `.tools/`, verifying SHA-256. **rclone 1.75.2 is built from official sources with a local read-only SMB security extension**, with a pinned module checksum, Go 1.26.10 and the `cmount` tag, without a C/C++ compiler. This avoids distributing the official binary built with Go 1.26.9, before the GO-2026-6809 fix. The global PATH is unchanged. Wails is pinned to **v3.0.0-beta.34**, alongside its matching JavaScript runtime. Libraries use `go.sum` and `package-lock.json`.

`scripts/engine-overlay.ps1` copies the verified rclone and pinned go-smb2 sources into `.tools/engine-overlay`, creates a private module replacement and maps the additional `.go.in` source files using Go's `-overlay` option. Go prohibits overlays inside the shared module cache, which remains unchanged. `bootstrap.ps1` rebuilds when the overlay digest changes. `scripts/engine-overlay.ps1 -Test` runs the focused library/backend extension tests and is included in `build.ps1 -Check`. Revalidate these additions when upgrading either upstream module.

Distribution retains `rclone-source.zip` as the verified upstream archive. The exact local additions and reproduction scripts are included in `SDrive-source.zip` under `scripts/`, and are also copied into `licenses/engine-overlay/` with the extension instructions. The pinned go-smb2 archive is included there as well. Reproduce the modified engine by extracting the SDrive source archive and running `scripts/bootstrap.ps1`; an unchanged upstream build does not contain the SMB security command.

The optional `TestSMBTransportNegotiationIntegration` requires `SDRIVE_TEST_SMB=1` and a disposable server with `SDRIVE_SMB_HOST`, `SDRIVE_SMB_USER`, `SDRIVE_SMB_PASSWORD`, `SDRIVE_SMB_ENCRYPTED_SHARE` and `SDRIVE_SMB_UNENCRYPTED_SHARE`. Domain and port can be supplied with `SDRIVE_SMB_DOMAIN` and `SDRIVE_SMB_PORT`. Use an account confined to synthetic fixture shares, one requiring encryption and one negotiating without it. Credentials belong only in the test process environment. Run `go test ./tests/integration -run '^TestSMBTransportNegotiationIntegration$' -count=1`. The test negotiates and checks both shares without writing remote files or mounting a Windows drive. Missing settings or an unavailable fixture fail explicitly after opt-in; without opt-in the test is reported as skipped. No fixture server is installed or configured by this test.

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

The native smoke test requires Windows, WebView2, the bootstrapped tools and a built frontend. It opens a visible, disposable Wails window and uses ordinary Playwright clicks/checks, including actionability checks. It checks all 19 provider options, authenticates against a read-only local WebDAV fixture, browses folders, cancels setup, saves a cache preference, switches all seven supported languages, verifies persisted preferences and minimum-window rendering for the additional locales, and exits safely. It does not use personal accounts, mount drives or change the user's Windows startup entry; the smoke build substitutes only the startup-write callback.

Each execution builds its own executable and creates its own profile under `.tools/ui-smoke/run-*`. Only its child receives the temporary `LOCALAPPDATA`; inherited `WEBVIEW2_*` overrides are removed. The debugger uses an ephemeral loopback port, discovered from that profile's `webview/EBWebView/DevToolsActivePort`. Only the explicitly linked smoke binary enables it; normal builds do not add debugger arguments. Images come directly from the connected WebView2, without launching another browser or assuming an Edge installation path. Repeated/concurrent runs can coexist with independent profiles.

Build/startup/scenario failures and handled SIGINT/SIGTERM interrupts enter cleanup. Cleanup stops only the launched application's PID and descendants, closes the local fixture and removes that execution's profile/executable. It has finite deadlines/retries and reports cleanup failure as a failed smoke. Reports and synthetic screenshots remain in `.tools/check-results/ui-smoke/run-*`; profiles, DPAPI keys and raw application output are not published. Forced termination of the harness or the machine can bypass cleanup; any leftover directory is named in the run output. Never point the harness at a personal profile.

The Windows workflow runs the native smoke as a required step after the build. Missing runtime, an unavailable graphical session, failed interaction and timeout fail the step; they never become successful skips. The hosted runner still needs to be validated by an actual workflow execution. Mounting remains separately opt-in and requires WinFsp.

```powershell
node --test scripts/ui-smoke-runtime.test.mjs
node scripts/check-results.mjs
```

`build.ps1 -Check` includes the harness regressions and records Go test events in `.tools/check-results/go-tests.jsonl`; packaging records the actual prerequisite signature tests separately in `signatures.jsonl`. The report distinguishes `passed`, `failed`, `skipped` (with the test's reason) and `not_run`. A passing signature parent cannot hide skipped component checks. Later packaging results supersede earlier results for the same signature test. Native smoke reports include every recorded local run, including deliberately injected failures; use their reasons to distinguish those experiments. The workflow always generates the summary and uploads only the summary, smoke result records and images, including on failure. Raw Go event files stay local. Local reports reflect the files currently present; CI uses a fresh checkout.

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

## Recovery and filesystem integration checks

The local fixture runs the pinned rclone as a disposable WebDAV server on an ephemeral loopback port. `TestServiceWebDAVIntegration` exercises `New`, `StartEngine`, guided setup, folder discovery, confirmation, access testing and reopening against isolated encrypted profiles. It also verifies a controlled upload gate, destination content and a downloaded SHA-256 hash, without mounting a drive:

```powershell
$previousEngineTest = $env:SDRIVE_TEST_ENGINE
try {
    $env:SDRIVE_TEST_ENGINE = '1'
    ./.tools/go/bin/go.exe test ./internal/desktop ./tests/integration ./internal/engine -count=1 -v -timeout 5m
} finally { $env:SDRIVE_TEST_ENGINE = $previousEngineTest }
```

Native checks require compatible WinFsp, a free letter between D and Z and at least 6 GiB free. They run sequentially and never install components, change startup preferences, use personal profiles/accounts or interrupt unrelated processes:

```powershell
$previousMountTest = $env:SDRIVE_TEST_MOUNT
try {
    $env:SDRIVE_TEST_MOUNT = '1'
    ./.tools/go/bin/go.exe test ./tests/integration -run 'TestWindowsMount|TestMountRecoveryChild' -count=1 -v -timeout 15m
} finally { $env:SDRIVE_TEST_MOUNT = $previousMountTest }
```

- `TestWindowsMountLifecycle` mounts through `Service`, verifies durable recovery protection and the pinned cache schema, and checks empty/64 MiB/Unicode files, rename, truncating replacement, deletion and read-only rejection. SHA-256 comparisons verify content; checks also require obsolete names to disappear and clean state to survive reopening. External creation/replacement/deletion uses explicit client `vfs/forget`; the fixture server disables directory caching. This does not certify automatic external-change detection.
- `TestWindowsMountPendingRecovery` covers network letters, local-disk letters and local folders using an alternate cache. It blocks uploads, waits for an upload attempt, verifies dirty metadata and the backing file hash, and terminates only its isolated helper. That helper owns the client's rclone kill-on-close job. After process termination and mountpoint release, the parent verifies preserved data, reopens the same profile and checks the saved connection/destination hash before recovering to the original fixture URL. It verifies uploaded content and clean disconnect.
- `TestWindowsMountOpenFiles` holds a writable handle during disconnect, observes the waiting stage, writes again and tries safe exit concurrently. After closing the handle and permitting uploads it verifies final content and clean disconnect/exit. The ordinary core test `TestExitDuringDisconnectPreservesLateOpenFileWrite` uses channels, a real temporary open handle and RPC doubles to force a write between the clean check and unmount completion; recovery, content and the original destination must survive.

Fixtures use unique, ACL-protected `.tools/integration/run-*` directories. Cleanup stops only owned engines and the local server, then removes successful fixtures. Failures retain synthetic data and `result.json` locally; do not attach keys, encrypted profiles or raw output to reports. Polling/RPCs and child termination have deadlines. Legacy checks skip missing prerequisites. The runtime/recovery checks fail when native mount opt-in is enabled but WinFsp, a disposable letter or required disk space is unavailable. The interruption helper requires both mount opt-in and the parent's isolated environment.

### Cache and mount runtime checks

`TestWindowsMountRuntimeModes` checks network/local drive types, owner ACLs, directory expiry/manual refresh with pending writes, property requests versus content reads, waiting for open files, folder collisions and reopening. For all three mount destinations it requires immediate permission errors when a read-only connection deletes files or empty directories, renames, overwrites or creates files/directories. It checks reading/traversal and intact content before and after refresh, no remote upload/move/delete requests, and successful overwriting, renaming and deletion after reconnecting with write access. `TestConnectionWorkerUsesSelectedCache` verifies the real worker cache path without a mount. See [runtime behavior](mount-runtime.md).

```powershell
$previousMountTest = $env:SDRIVE_TEST_MOUNT
try {
    $env:SDRIVE_TEST_MOUNT = '1'
    ./.tools/go/bin/go.exe test ./tests/integration -run '^TestWindowsMountRuntimeModes$|^TestWindowsMountPendingRecovery$' -count=1 -v -timeout 5m
    if ($LASTEXITCODE) { throw 'Native mount runtime validation failed' }
} finally { $env:SDRIVE_TEST_MOUNT = $previousMountTest }
```

`node scripts/ui-smoke.mjs` checks native WebView2 folder-destination controls, cache location/timing, localization and safe exit. With process-scoped `SDRIVE_TEST_MOUNT=1`, the scenario also connects a disposable local folder through the actual Wails bridge, refreshes it and disconnects. Without that opt-in it cancels setup before mounting. Neither scenario installs components or uses personal accounts.

### Regression inventory

The integration suite reuses these existing behavioral regressions:

| Behavior | Regression evidence |
| --- | --- |
| Settings completeness and cleanup | `TestIncompleteSettingsNeverAuthorizeCleanupOrWrites`, `TestCleanupFailureIsRecoverableAndRetryClearsWarning` |
| Cache metadata and counters | `TestCacheMetadataRequiresExplicitBoolean`, `TestCriticalCacheStatsValidation` |
| Cancellation and late responses | `TestSlowFolderCallsAllowCancellationAndRejectLateResults`, `TestCancellationWaitsForLateConfigurationBeforeDeleting`, engine job/cancellation tests |
| FTP renewal and transport | `TestFTPRenewalRestoresTransportAndPreservesRecovery`, transport persistence in `TestConfigurationJobProtocolWithRclone` |
| Unknown upload state | `TestSnapshotUnknownAndSampleExpiry`, frontend unknown-upload tests |
| Automatic connection failures | `TestAutoMountReportsEarlyFailuresAndContinues`, frontend automatic-connection error test |
| Profile ownership | `TestDataDirOwnershipAcrossProcessesAndCrash`, legacy installer marker regression |
| Recovery destination binding | `TestRecoveryRejectsDestinationChanges`, `TestRecoveryRejectsChangedLibraryButAllowsRenewedTokens`, `TestRecoveryRenewalVerifiesLibraryBeforeSuccess` |
| Setup completion validation | `TestSetupRejectsInvalidCompletionAndSavedRemote`, `TestConfiguredRemoteRequiresOAuthOrAlternateCredentials`, real-engine response tests |

Recovery coverage proves configured destination binding, not universal account ownership; [identity limits](provider-validation.md#recovery-identity-limits) still apply. Frontend contracts remain mocked bridge checks. The frontend suite includes a complete typed API mock, SSH probe failure/retry with explicit fingerprint trust and trust reset after server/port changes, and engine restart failure, duplicate suppression, retry and retained recovery. Existing access tests remain in place.

Run engine, frontend and native checks separately. Passing unit tests does not certify authenticated providers, native WebView behavior, power-loss recovery or installer execution.

## Desktop lifecycle checks

Desktop regressions run without provider accounts, mounts or changes to the real startup registry:

```powershell
./.tools/go/bin/go.exe test ./internal/desktop ./tests/integration ./internal/core ./internal/platform ./internal/i18n
npm --prefix frontend test
npm --prefix frontend run build
```

The desktop tests start a disposable child process with the real Wails message-only receiver and verify activation while preserving profile ownership. Platform tests cover independent profile identities and the legacy installer marker. Other regressions cover stale snapshot delivery, alert deduplication, installed/running version reporting, negative/off-screen/oversized geometry and small monitor work areas. Frontend tests cover About, translation, visibility handshakes, suspended polling and obsolete responses.

`About()` adds component-version diagnostics to the service bridge. `DesktopVisible()` supplies initial native visibility; the `desktop:visibility` event carries a boolean for subsequent changes. Native visibility includes minimization. The interface polls every three seconds when visible and refreshes immediately on return. While hidden, a native loop samples the existing `Snapshot` every 15 seconds; only current snapshots reach the tray. Active authentication/setup status checks retain their existing behavior.

Drive disconnection publishes lifecycle notifications directly to a separate desktop overlay, including manual, safe-exit and inactivity paths. The overlay remains above ordinary windows without retaining foreground focus, shows checking/waiting/unmounting stages and expires successful results after three seconds. Failures remain until dismissed or opened in SDrive; dismissing a result never cancels an active operation. Locked presentation contains no drive names, destinations or error details. Notifications are transient and do not change the bridge's disconnect methods or persisted settings.

Run focused regressions with `./.tools/go/bin/go.exe test ./internal/desktop ./tests/integration ./internal/core ./internal/i18n -run 'TestDisconnect|TestConcurrentDisconnect|TestLocale'` and `npm --prefix frontend test -- src/DisconnectOverlay.test.tsx`. For the real Windows/WebView2 check, set `SDRIVE_TEST_DESKTOP=1` only in the test process environment and run `./.tools/go/bin/go.exe test ./internal/desktop -run '^TestDisconnectOverlayNative$' -count=1`. It uses a child process and temporary profiles, checks real rendering, focus preservation, minimization, simultaneous progress, English/Portuguese text, lock redaction, success expiry and the close button. Missing WebView2 or an unavailable graphical session fails the opted-in check. It does not mount drives, use provider accounts or certify physical multi-monitor/DPI and screen-reader behavior.

Tray regression checks must include a real right-click, including in the hidden-icons overflow, reopening the menu and selecting **Exit safely** in both supported languages. Windows can deliver another tray notification inside the popup's message loop; it must not replace or destroy the active menu. Sending a single synthetic notification does not reproduce this case. `TestTrayRepeatedOpenPreservesActiveMenu` covers preservation of the menu and its drive submenu during a repeated notification.

For native acceptance, use a new temporary profile by setting `LOCALAPPDATA` only in the child process environment. Check background startup, second launch and focus, close-to-tray, minimization, geometry after restart, About and safe exit. Stop only that profile's own rclone child to verify that a new background engine failure reveals the window and an unchanged failure does not reveal it again. Verify taskbar/DPI and monitor-removal behavior separately on suitable hardware. Do not reuse a personal profile or terminate another instance to run these checks. Use the native UI smoke harness separately.

## Diagnostic checks

`Service.Diagnostics()` returns a sanitized text report through the existing Wails bridge. The About dialog previews it and uses the installed runtime's `Clipboard.SetText` to copy exactly that preview. Collection is installed before service publication and engine initialization; tests can leave the collector nil. The engine and service share one collector, while its disk worker never takes service or engine locks. Snapshot reads do not emit routine status events; repeated engine failure categories are deduplicated.

Run the desktop, integration and affected-package tests, frontend checks and production build:

```powershell
./.tools/go/bin/go.exe test ./internal/desktop ./tests/integration ./internal/diagnostics ./internal/engine ./internal/core ./internal/platform ./internal/i18n
./.tools/go/bin/go.exe vet ./...
npm --prefix frontend test
npm --prefix frontend run build
./.tools/go/bin/go.exe build -tags production -o .tools/diagnostics-check.exe .
```

Diagnostics tests use temporary profiles, injected errors, fake RPC responses and disposable helper child processes. They exercise redaction, malformed/oversized history, file rotation, bounded memory/queue, concurrent recording, unavailable storage and bounded writer shutdown. Engine helper tests cover early and unexpected exit, graceful shutdown, forced termination and the final unterminated stderr record. Oversized/malformed stderr is drained without retaining it or blocking readiness. Engine stderr copying completes before exit context is published; `WaitDelay` bounds inherited pipes after process termination. These helpers do not load real credentials or mount drives.

Frontend regressions cover explicit preview/copy, exact copied text, clipboard and loading failures, retry, English/Portuguese controls, settings-recovery availability and late responses after closing. They mock the clipboard and bridge. To validate native clipboard behavior separately, use a disposable profile, inspect the preview and paste it into a temporary editor; check keyboard selection/scrolling and focus at the minimum window size in both languages. Never attach a personal profile or raw logs to test evidence. Native WebView, actual clipboard integration and interrupted-update VM acceptance must be reported separately from unit tests and build success.

## Accessibility checks

The four application dialogs use native `dialog.showModal()` in WebView2. The browser supplies modal background isolation and keyboard navigation; cancellation remains controlled by the application so busy operations and asynchronous authentication cleanup cannot be bypassed with Escape. Closing returns focus to the opener when it still exists. Action disclosures retain ordinary Tab navigation and close on an action, outside pointer interaction, focus departure or Escape; actions and Escape return focus to the summary.

Frontend tests cover dialog cancellation/busy guards, help/error associations (including secrets), localized labels, menu behavior and deduplicated state announcements. JSDOM only stubs dialog opening/closing; it does not establish native isolation, focus traversal or screen-reader speech. Run `npm --prefix frontend test`, `npm --prefix frontend run build` and `./.tools/go/bin/go.exe test ./internal/i18n`.

For native checks, use a newly generated temporary profile with child-process-scoped `LOCALAPPDATA`, as described above. Do not use personal accounts, provider credentials or mounts. Verify:

- Open Preferences, About, connection setup and settings restoration using the keyboard. Tab/Shift+Tab must not reach background controls; Escape must respect busy state and cleanup. If two application dialogs are deliberately mounted for testing, only the top one accepts input and cancellation. Inspect WebView2's accessibility tree for background exclusion, then check navigation and announcements with a Windows screen reader separately.
- At the 850 × 620 minimum window, check 125%, 150% and 200% **WebView zoom** in both languages, including long connection names/paths, expanded advanced fields and menus. Dialogs scroll internally, all actions remain reachable, and narrow layouts reflow without horizontal clipping. WebView zoom is separate from Windows display scaling; repeat on hardware at the relevant DPI settings.
- Emulate `forced-colors: active` in the native WebView and inspect focus, disabled controls and textual status. Then repeat with an actual Windows contrast theme; emulation does not validate the operating system's palette or screen-reader behavior. Do not change workstation-wide preferences automatically.

Keep screenshots/results separate from credentials and record which checks actually ran. CDP screenshots at native WebView zoom can be cropped or incorrectly scaled; use a capture of only the isolated native window when assessing those zoom results. Stop only the isolated application and its children, and verify profile cleanup.

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

## Folder navigation and validation

Connection setup lives in `frontend/src/connection/`: `useConnectionWizard` owns draft data, requests and commands; its reducer distinguishes service/access/edit, authentication with a session, folder selection with an optional saved connection, and completion with a saved connection after successful mounting. Credential fields, provider questions and folder navigation have separate views. Cancellation still waits for backend cleanup, failed mounting reuses the saved connection, and obsolete folder/status results cannot move the wizard backward. Shared modal behavior remains in `useDialog`.

Core configuration sessions, preferences and engine lifecycle have separate files in the same package. All retain the `*Service` receiver, the existing mutex and operation/session ownership rules. Moving these responsibilities does not introduce another service or lock.

Drive names contain 1–80 Unicode code points after trimming Go-compatible Unicode whitespace. Both frontend and backend apply this rule; ASCII, accented letters and simple emoji each count as one code point, while combining sequences and joined emoji count as multiple. Names are not truncated or normalized. The frontend permits the provider-name default for a new blank name; saved edits require a nonempty name. Go validates persisted connections too. JavaScript trimming explicitly matches `strings.TrimSpace`, including NEL and excluding BOM.

### Bridge compatibility checks

`frontend/src/bridge-fixtures.json` contains synthetic serialized Go samples and service method signatures. The core contract test compares actual Go serialization and reflected signatures to this fixture. Frontend contract tests check client method names/arguments/results and TypeScript field names, required fields and nullable metrics against the same samples. The frontend build runs the compile-time assertions; running Vitest alone does not type-check them. Review Go types, TypeScript types and fixtures together when a contract intentionally changes. Never put account data or credentials into fixtures.

Run `./.tools/go/bin/go.exe test ./internal/desktop ./tests/integration ./internal/core ./internal/platform ./internal/i18n`, `./.tools/go/bin/go.exe vet ./...`, `npm --prefix frontend test` and `npm --prefix frontend run build`. The desktop package tests include the isolated real-engine self-check. For the changed concurrency paths, use the existing Windows compiler setup with `CGO_ENABLED=1` and `go test -race ./internal/core ./internal/i18n`. Production compilation can target `.tools/validation-check.exe` without replacing the installed application. These checks do not mount drives or establish native WebView/provider behavior.

### Folder interactions

The folder picker retries a failed listing in the same authenticated draft. Breadcrumbs and the root action change the selected destination; the filter only matches names in the current listing and never searches remotely. Direct text must be applied with **Open destination** before connecting. A denied root listing does not prevent confirming a directly entered destination that the account can list. Recovery and saved drafts lock destination changes.

Validation errors from the existing service methods retain their error message and serialize `{field, message}` into Wails `RuntimeError.cause`. The frontend associates these errors with the corresponding input, opens containing options and focuses the field without clearing entered values. Provider/network errors remain general messages; validation errors never include the submitted value. Go remains responsible for path traversal protection and validates host/port before SSH probing.

Core regressions cover retry, restricted-root access, unsafe paths rejected before RPC, recovery locks, host/port/URL validation and error serialization. Frontend regressions cover breadcrumbs, local filtering, stale responses, duplicate retries, unapplied paths, saved/recovery locks, field accessibility and English/Portuguese feedback. The runtime test uses the installed Wails `RuntimeError` class; it does not exercise a native WebView bridge. Run the core/i18n tests and vet, frontend tests/build and a production build. These checks use temporary profiles and RPC doubles; they do not authenticate live accounts or mount drives.

## Sign-in identities

Authentication regressions also verify strict completion response fields against the pinned local rclone binary, all three FTP transports and password obscuring in a temporary encrypted profile. The FTP configuration checks do not contact a storage server. Setup verifies saved remotes before success; frontend tests cover failed access-detail loading, renewed transport/consent, sensitive-question masking, show/hide controls and service-account availability. `ConnectionFields` returns `{values, transport}` to the frontend; `values` includes only non-secret catalog fields. See the [provider identity limits](provider-validation.md#recovery-identity-limits) before describing these checks as account verification.

OAuth registration belongs to the user or their organization, not SDrive maintainers. No registration file or shared client is embedded in the build. Follow [OAuth setup](oauth-setup.md) for the supported account/redirect/permission flows, and keep credentials out of source and fixtures.

Connection supervision uses isolated per-remote processes while preserving the legacy encrypted source and cache names. Each persistent worker owns its encrypted file; session-only workers use `--config NUL`. Proxy secrets live in `proxy.enc`, encrypted with the DPAPI-backed profile key, and are omitted from bridge views. Read [connectivity](connectivity.md) before changing process routing, cache identity, authentication storage or protocol controls.

Focused connection checks:

```powershell
./.tools/go/bin/go.exe test ./internal/core ./internal/engine ./internal/platform ./internal/i18n
npm --prefix frontend test
npm --prefix frontend run build
$env:SDRIVE_TEST_ENGINE = '1'
try { ./.tools/go/bin/go.exe test ./tests/integration ./internal/core ./internal/engine -run '^Test(ConnectionEngine|SessionOnlyEngine|ProviderProxy|SOCKSProxy)' -count=1 -v }
finally { Remove-Item Env:SDRIVE_TEST_ENGINE }
```

These real-engine tests create disposable loopback WebDAV storage, independent limited client processes, an authenticated rejecting HTTP proxy, disposable loopback SOCKS5 proxies, encrypted isolated profiles and in-memory credential workers. They check transfer contents/timing, proxy/RPC separation, legacy import and reopening. SOCKS5 checks cover both URL schemes, optional authentication, proxy-side destination DNS, failed credentials, unavailable proxies without direct fallback, reconnected transfers, HTTPS WebDAV identity verification and credential redaction. The fixtures need no external proxy or provider account. They do not access real provider accounts, launch a personal browser or mount drives. Use the existing native UI smoke harness separately. Live OAuth setup, cancellation and renewal require provider-specific validation. Legacy charset conversion is unsupported by the pinned engine.

## Online and offline installers

`installer/prerequisites.json` is the single source for addresses, SHA-256 values and publishers of official WinFsp and WebView2 x64 installers. The manifest is embedded in the application. `scripts/prerequisites.ps1`, called during packaging, downloads files for the offline package, checks hashes and signatures and stops packaging on mismatch. Packaging also runs the native Windows signature test against both official files. It does not automatically switch to newer versions: when updating the manifest, validate origin, signature and compatibility.

`scripts/package.ps1` reads the release version from `frontend/package.json` and passes `/DAPP_VERSION` to the same NSIS script twice: for `SDrive-<version>-setup.exe` (online), and with `/DOFFLINE` for `SDrive-<version>-offline-setup.exe`. The Windows resources, application version and CI installer paths use that same package version. Keep the package lockfile synchronized when changing the package version; do not hard-code a release version in scripts, workflows or installer instructions. CI delivers both installers and their checksums. SDrive, rclone, sources and licenses are included in both.

The build compiles `assets/sdrive.rc` into the ignored `resource_windows_amd64.syso` before compiling Go. The resource template uses the same package version and embeds `assets/sdrive.ico` as resource 3, which Wails uses for window and taskbar icons. The compiler comes from the same pinned, checksum-verified w64devkit used by the race checks. Installer and uninstaller icons share the ICO; tray status colors remain independent. After building, `scripts/test-windows-resources.ps1` verifies the application and diagnostic executable metadata and all seven icon frames against the source ICO; `-IncludeInstallers` also checks both installers. The source archive includes the assets and resource scripts for rebuilding.

The online installer first checks installed components and calls the Go helper to download only those needed, using the same pinned official files as the offline variant. Users do not need PowerShell, download plugins or scripts. The helper accepts only `winfsp` and `webview2` IDs; URLs are not accepted through the command line or Wails bridge.

Downloads use HTTPS, reject HTTP redirects, have a 512 MiB per-file limit, a 30-minute total timeout and cancellation after 60 seconds without receiving data. Partial files are removed on error; there is no permanent cache. English progress is sent as UTF-16 to the native NSIS log. Errors offer retry or cancellation. An already verified file in the temporary directory is reused during the same installation.

Both variants verify SHA-256, Authenticode signature and publisher CN through native Windows APIs before executing each component, and keep the file locked against writes and deletion until execution finishes. Trust verification uses local certificates and does not require online revocation queries, preserving offline installation.

The package runs without elevation, preserves the original user and elevates only the WinFsp MSI. Exit codes 0 and 3010 mean success and success with restart pending. Authorization cancellation or installation failure prevents reporting completion. WebView2 is installed using `/silent /install`, following [Microsoft documentation](https://learn.microsoft.com/microsoft-edge/webview2/concepts/distribution#offline-deployment).

Uninstallation offers **Retry / Cancel** for an active data lock or legacy mutex, failed Explorer icon cleanup, and failed removal of owned files or Windows registration. Close SDrive using **Exit safely**, resolve the reported blocker and retry in the same window. Retrying an initial check or icon cleanup repeats the safety checks; retrying removal repeats only the failed operation. Handles opened by the checks are closed before prompting. A mismatched installation directory remains a terminal error. Silent uninstallation (`/S`) returns exit code 1 on these failures without prompting or looping; successful removal returns 0. Custom installer and uninstaller messages support all seven [interface languages](localization.md), selected by the Windows language with English as the default. This selection is independent of the saved application language. Technical helper progress remains in English.

The uninstall entry is removed after owned files have been deleted. User configuration, credentials, cache and shared prerequisites remain untouched; unowned files in the installation root are retained. Cancellation after deletion starts can leave a partially removed installation; there is no rollback. Repair by reinstalling if a later uninstall session cannot continue.

Run `./scripts/test-uninstall.ps1` for focused native NSIS regression checks (requires the existing compiler in `.tools/nsis/nsis-3.12`, or pass `-Compiler`). The harness compiles the production uninstall section with isolated paths, a temporary registry key and a fake icon-cleanup executable. It exercises real file locks, the legacy mutex, repeated retries, cancellation, icon-cleanup failures, directory validation and silent failures in all seven supported languages. Only fixture dialogs are automated; the personal installation and shared components are not used.

## Installer acceptance on a clean machine

Run these scenarios separately in disposable Windows 10 22H2 (build 19045) x64 and Windows 11 x64 VMs; do not remove shared components from your workstation to simulate their absence. Both systems use the same online and offline installers:

- Install offline without WinFsp/WebView2 and without internet access; open the wizard after completion or restart when requested.
- Install online with both absent; watch progress and verify both dependencies are prepared automatically.
- Install online with both present and no internet; no download should occur.
- Interrupt the network during download, retry and cancel; no partial file may execute and the existing application must be preserved.
- Repeat with both present and with only one present; confirm compatible components are reused.
- Use a standard user and authorize with another administrator; data and credentials must stay in the original user's profile.
- Cancel authorization, simulate MSI failure and test exit code 3010; no automatic restart or false success.
- Run the installer again for repair, update while preserving connections/cache, and uninstall while preserving data/shared components.
- In both languages, start uninstalling with SDrive running, retry while it is still running, exit safely and retry successfully in the same window. Repeat with cancellation and an owned file held open; confirm `/S` fails without a prompt when blocked. These product-level VM checks complement the isolated regression harness.
- Confirm users do not need to run terminals or commands.

These scenarios require the VM and UAC interaction. Compiling NSIS and running mocked tests do not replace this validation.

Run `./scripts/test-installer-requirements.ps1` for native NSIS checks of the production platform gate with injected build and architecture values. It accepts Windows 10 build 19045 and Windows 11 builds, rejects older or invalid builds and non-AMD64 architectures, and verifies that rejected platforms never reach installation. This fixture does not establish runtime compatibility on a different Windows version.

Windows 10 22H2 clean-machine acceptance remains pending until results from that system are recorded. WinFsp 2.1 and Microsoft WebView2 are shared prerequisites; their existing integrity checks and automatic installation apply to both supported systems. See [Microsoft's WebView2 support policy](https://learn.microsoft.com/en-us/deployedge/microsoft-edge-supported-operating-systems).
