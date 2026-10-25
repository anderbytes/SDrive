# SDrive

Open source desktop application that mounts remote storage as drives in Explorer. Windows 11 x64, Go, Wails v3, React/TypeScript and rclone. Licensed under **GPL-3.0-only**.

## Install and connect

1. Download and run **SDrive-0.1.0-setup.exe**, the recommended online installer. It downloads and prepares missing components automatically; no scripts or separate tools are needed. Allow Windows to prepare virtual drives if prompted. Restart only if the installer requests it.
2. Open SDrive and choose **New connection**. Select a service and continue.
3. Sign in through your browser or enter the credentials provided by your storage administrator.
4. Choose a folder and click **Connect drive**. Access is checked automatically. Use **Open in Explorer** to access your files.

The drive name and letter are suggested automatically. **Basic** contains password/address access, object-storage credentials and GCS service accounts. **Advanced** contains OAuth methods requiring your own application registration, Client ID and Client Secret; follow the [service setup instructions](docs/oauth-setup.md). Technical connection options remain separate. See [authentication and connectivity](docs/connectivity.md) for browser choice, session-only access, per-drive bandwidth, proxy and verified WebDAV fallback, including their supported-service limits.

The online installer includes SDrive and rclone. Compatible WinFsp and WebView2 installations are reused; only missing or incompatible components are downloaded. Follow progress in the installer window. If a download fails, choose **Retry** or **Cancel**.

For installation without internet access or on networks that restrict downloads, use **SDrive-0.1.0-offline-setup.exe**, which includes all components. Signing in and accessing remote files requires connectivity with either version. Run either installer again to repair missing components. The application runs without administrator privileges.

Before disconnecting or exiting, close files and wait for uploads. Closing the window keeps drives active in the tray. **Preferences** lets you start SDrive with Windows; automatic mounting is optional per connection. Both are disabled initially. Preferences groups settings into General, Appearance, Cache, Network and Configuration tabs. Under Appearance, choose System (the default), Light or Dark. Theme changes preview immediately; Save keeps the choice, while Cancel or Escape restores the saved theme. System follows Windows theme changes, including on the lock screen.

Choose network-drive or local-disk presentation, or mount into a new local subfolder. Preferences includes safe cache-directory changes and retention/upload timing. Drive options include metadata lifetime, explicit Explorer refresh and provider-reported capacity. See [cache, mounting and browsing](docs/mount-runtime.md) for defaults and limits.

The tray shows connected-drive counts and distinguishes uploads, recovery, unknown status and failures. Right-click it to connect, disconnect or open a drive, or exit safely. Launching SDrive again opens the existing window in the same Windows session. New background problems open the window once; an unchanged problem does not repeatedly interrupt you. Background status checks run every 15 seconds while visual polling is suspended.

Window position, size and maximization are remembered and adjusted to available monitors. **About SDrive** lists application, running rclone, Windows and installed WinFsp/WebView2 versions; unavailable components are shown explicitly.

**Preferences → Configuration → Portable configuration** exports credential-free connection definitions and reviews imports as drafts requiring authentication. Optional Explorer opening and per-user service icons default off. Connection cards show configured options alongside upload/recovery status. See [configuration portability and desktop preferences](docs/portability-and-desktop.md).

## Update, repair and collect diagnostics

1. Before updating, close files in other applications, let uploads finish and choose **Exit safely**. If exit is blocked, complete recovery at the original destination first. Closing the window only hides SDrive in the tray.
2. Run the compatible online or offline installer for your Windows installation. Both preserve your profile; the offline installer is useful when prerequisite downloads are restricted. Updates are manual. Do not run the installer while SDrive is active.
3. Reopen SDrive and check **About SDrive**. If installation fails or components are missing, preserve the profile and rerun the compatible installer to repair it. Installer failure/interruption is not a successful update; replacement is not guaranteed to be transactional.

If SDrive reports an unsupported settings version, use the application version that created it or a compatible newer version. Do not downgrade by deleting or editing settings, restore an older backup to bypass incompatibility, or remove keys/cache/recovery markers. For damaged preferences, use the application's deliberate recovery flow described in [local settings recovery](docs/security.md#local-settings-recovery).

For support, open **About SDrive → View diagnostics**, review the technical text, then choose **Copy report** and paste it into your support request. This remains available when settings or the engine cannot load. The controls follow your selected language; the technical report is in English. Include what you were doing and whether the issue repeats, without adding private names, destinations or files.

Minimal technical events are collected automatically and kept only in your local profile: at most 256 events in memory and two rotating files of 128 KiB each. There is no automatic transmission or telemetry. Reports contain versions, aggregate state, failure stages and sanitized engine-exit context; they exclude credentials, account/connection names, URLs, private paths and file contents. Unknown causes remain generic. Storage failures never authorize cache deletion or bypass safe exit. See [diagnostic privacy and limits](docs/security.md#local-diagnostics).

## Language

English is the application's default and the language of the repository, source messages and documentation. The interface supports **English** and **Português (Brasil)**. Choose **Preferences → General → Language** and save. The choice is stored in your settings and applies to the interface, connection wizard and tray menu. Missing translations fall back to English. Account names, remote paths and file contents are preserved.

## Development

See the [maintainer guide](docs/development.md) to build and test, and the [localization guide](docs/localization.md) to add interface languages. Building is not required to install the application. OAuth registrations belong to each user or their organization; SDrive supplies no shared client.

## Project status

All 18 profiles have implemented setup flows and automated tests. This **does not certify compatibility with every account and provider**. See the [validation matrix](docs/provider-validation.md) and [contributor guide](docs/contributing.md). Proton Drive and Wails v3 are beta.

The first version does not include Linux support, full offline synchronization, distributed editing locks, a Windows service or an automatic updater. Files are accessed on demand; saving to a virtual drive does not mean the upload has finished.

## Data and recovery

`%LOCALAPPDATA%\SDrive` contains preferences, encrypted legacy and per-connection rclone configurations, DPAPI-protected keys, encrypted proxy preferences and the cache-location binding. The cache defaults to this profile and can be changed in Preferences after clean disconnection. Profile/cache directory permissions are restricted to the current user and SYSTEM. SDrive uses its own configuration and does not change your regular `rclone.conf`.

Never delete the cache while recovery is pending. After an interruption, use **Recover** on the original connection. To renew credentials during recovery, use **Reconfigure account**, preserving the server, username, folder and access mode. Destination changes and removal remain blocked until recovery finishes.

Uninstalling preserves data, credentials and cache to prevent loss of pending uploads. After resolving all pending files, you can remove that directory manually. The DPAPI key depends on your Windows user; copying files to another account is not a portable credential backup.

Read the [security model and limitations](docs/security.md) before using sensitive data. Do not publish configurations, tokens, keys, dumps or cache files in issues.

Application PIN protection, safe idle disconnect and Windows file-access queries: [access and file operations](docs/access-and-file-operations.md). The lock protects management controls; connected drives remain accessible in Explorer.
