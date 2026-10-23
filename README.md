# SDrive

Open source desktop application that mounts remote storage as drives in Explorer. Windows 11 x64, Go, Wails v3, React/TypeScript and rclone. Licensed under **GPL-3.0-only**.

## Install and connect

1. Download and run **SDrive-0.1.0-setup.exe**, the recommended online installer. It downloads and prepares missing components automatically; no scripts or separate tools are needed. Allow Windows to prepare virtual drives if prompted. Restart only if the installer requests it.
2. Open SDrive and choose **New connection**. Select a service and continue.
3. Sign in through your browser or enter the credentials provided by your storage administrator.
4. Choose a folder and click **Connect drive**. Access is checked automatically. Use **Open in Explorer** to access your files.

The drive name and letter are suggested automatically. Technical settings are under **Advanced options**. The standard flow does not require knowledge of rclone, editing files or creating a Client ID. Sign-in is unavailable when this build has no OAuth application registration for the selected service.

The online installer includes SDrive and rclone. Compatible WinFsp and WebView2 installations are reused; only missing or incompatible components are downloaded. Follow progress in the installer window. If a download fails, choose **Retry** or **Cancel**.

For installation without internet access or on networks that restrict downloads, use **SDrive-0.1.0-offline-setup.exe**, which includes all components. Signing in and accessing remote files requires connectivity with either version. Run either installer again to repair missing components. The application runs without administrator privileges.

Before disconnecting or exiting, close files and wait for uploads. Closing the window keeps drives active in the tray. **Preferences** lets you start SDrive with Windows; automatic mounting is optional per connection. Both are disabled initially.

## Language

English is the application's default and the language of the repository, source messages and documentation. The interface supports **English** and **Português (Brasil)**. Choose **Preferences → Language** and save. The choice is stored in your settings and applies to the interface, connection wizard and tray menu. Missing translations fall back to English. Account names, remote paths and file contents are preserved.

## Development

See the [maintainer guide](docs/development.md) to build, test and configure sign-in registrations, and the [localization guide](docs/localization.md) to add interface languages. These steps are not required to install or use the application.

## Project status

All 18 profiles have implemented setup flows and automated tests. This **does not certify compatibility with every account and provider**. See the [validation matrix](docs/provider-validation.md) and [contributor guide](docs/contributing.md). Proton Drive and Wails v3 are beta.

The first version does not include Linux support, full offline synchronization, distributed editing locks, a Windows service or an automatic updater. Files are accessed on demand; saving to a virtual drive does not mean the upload has finished.

## Data and recovery

`%LOCALAPPDATA%\SDrive` contains preferences, encrypted rclone configuration, a DPAPI-protected key and cache. Directory permissions are restricted to the current user and SYSTEM. SDrive uses its own configuration and does not change your regular `rclone.conf`.

Never delete the cache while recovery is pending. After an interruption, use **Recover** on the original connection. To renew credentials during recovery, use **Reconfigure account**, preserving the server, username, folder and access mode. Destination changes and removal remain blocked until recovery finishes.

Uninstalling preserves data, credentials and cache to prevent loss of pending uploads. After resolving all pending files, you can remove that directory manually. The DPAPI key depends on your Windows user; copying files to another account is not a portable credential backup.

Read the [security model and limitations](docs/security.md) before using sensitive data. Do not publish configurations, tokens, keys, dumps or cache files in issues.
