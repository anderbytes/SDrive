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

## Sign-in identities

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
