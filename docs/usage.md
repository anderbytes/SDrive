# SDrive user guide

This guide covers everyday use on Windows. For a quick introduction, see the [README](../README.md). Installing SDrive does not require building it, running scripts or installing developer tools.

## Install and connect

Choose an installer from the repository's Releases page:

- **Online installer:** recommended for most users. It includes SDrive and its storage engine and downloads missing Windows components automatically.
- **Offline installer:** includes the same application and all required components for installation without internet access. Signing in and accessing remote storage still require connectivity.

Compatible WinFsp and WebView2 installations are reused. Follow the progress in the installer window. If a download fails, choose **Retry** or **Cancel**. Allow Windows to prepare virtual drives if prompted, and restart only if the installer requests it. The application runs without administrator privileges; preparing the drive component can require administrator approval.

Open SDrive and choose **New connection**. Select a service, sign in or supply your access details, then choose a remote folder and connect it. SDrive suggests a drive name and letter and checks access automatically. Choose **Open in Explorer** to browse your files.

**Basic** contains password/address access, object-storage credentials and Google Cloud Storage service accounts. **Advanced** contains OAuth methods that require your own application registration, Client ID and Client Secret. SDrive does not supply shared application credentials. Follow the [sign-in setup instructions](oauth-setup.md) for your service; your provider or organization may require extra permissions or approval. Technical connection options remain separate.

The [connection guide](connectivity.md) explains browser choice, session-only access, transfer limits, proxies and the limits of individual services. Available setup flows and automated checks do not establish compatibility with every live account; see the [validation matrix](provider-validation.md).

## Use your drives

Open, edit and save files in Explorer or your usual applications. Files are accessed on demand; SDrive is not a full offline synchronization or backup tool. Saving a file locally does not confirm that its remote upload has finished.

Before disconnecting or exiting, close open files and wait for uploads. If SDrive reports pending recovery or cannot confirm that files are safe, resolve the condition before exiting. **Closing the window keeps drives active in the tray.** Right-click the tray icon to open SDrive, connect or disconnect a drive, or choose **Exit safely**.

Cards show connection status and, when available, remote storage usage. Choose **… → Drive details** for upload, cache, security and recovery information. Capacity may describe the account rather than the selected folder. Missing information is shown as unavailable or unknown.

You can choose network-drive or local-disk presentation, or mount into a new local subfolder. Read-only access and automatic connection are configured per drive. Automatic connection is optional and disabled initially. See [mounting, cache and browsing](mount-runtime.md) for limits and details, including refreshing files changed elsewhere.

## Personalize SDrive

Open **Preferences** and use its section menu. In narrow windows, expand the current section button to navigate.

- **General:** choose the interface language and whether SDrive starts with Windows. Startup is disabled initially and does not enable automatic connection by itself.
- **Appearance:** choose System, Light or Dark. Changes preview immediately; **Save** keeps them, while **Cancel** or Escape restores the saved theme. Individual drive cards can also be personalized.
- **Cache:** review cache size, retention, upload timing and location. Change location through SDrive only after clean disconnection; do not move or delete the cache manually.
- **Network:** configure a proxy when your network requires one. Individual connection options include transfer limits.
- **Configuration:** export credential-free connection definitions or import them as drafts. Imported connections require authentication before use.

The interface supports English, Brazilian Portuguese, Spanish, French, German, Italian and Simplified Chinese. English is the default and fallback. The language choice applies to the interface, connection setup and tray. Installers and the uninstaller follow the Windows language when supported, independently of the saved application preference. Third-party components and technical logs may use their own language.

SDrive remembers window position, size and maximization and adjusts them to the available monitors. Launching it again opens the existing window in the same Windows session. New background problems can bring the window forward; an unchanged problem does not repeatedly interrupt you.

For portable configuration and optional Explorer integration, see [configuration and desktop preferences](portability-and-desktop.md). For application PIN protection, safe idle disconnection and file-access queries, see [access and file operations](access-and-file-operations.md). The application lock protects management controls; connected drives remain accessible in Explorer.

## Update or repair

Updates are manual:

1. Close files in other applications, wait for uploads and choose **Exit safely**. If exit is blocked, complete recovery at the original destination first. Closing the window only hides SDrive in the tray.
2. Run a compatible online or offline installer. Both preserve your profile. Do not run the installer while SDrive is active.
3. Reopen SDrive and check **About SDrive** for the installed and running component information. If installation fails or components are missing, preserve the profile and rerun the compatible installer to repair it.

An interrupted or failed installation is not a successful update; replacement is not guaranteed to be transactional. The offline installer can help when prerequisite downloads are restricted.

If SDrive reports an unsupported settings version, use the application that created it or a compatible newer release. Do not delete or edit settings, restore an older backup to bypass incompatibility, or remove keys, cache or recovery markers. For damaged preferences, follow [local settings recovery](security.md#local-settings-recovery).

## Collect diagnostics and report a problem

Open **About SDrive → View diagnostics**, review the text, then choose **Copy report**. This remains available when settings or the storage engine cannot load. The controls follow your chosen language; the technical report is in English.

Use the repository's bug report form. Include the installed version, environment, steps to reproduce, what you expected and what happened. Remove private names, destinations, paths and file contents from any additional text or screenshots. Never attach your profile, configuration, tokens, keys, dumps or cache files.

Diagnostics stay on your computer; SDrive sends no telemetry or automatic reports. The report includes component information, aggregate state and sanitized failure context, excluding credentials and private file content. Unknown causes remain generic. Collection is bounded and may miss events; it is a troubleshooting aid, not an audit trail. See [diagnostic privacy and limits](security.md#local-diagnostics).

The maintainer cannot personally test every connection type, feature or environment. Community reports and tests with disposable data help improve SDrive. See [Contributing](CONTRIBUTING.md) for ways to help without exposing private information or risking important files.

## Recover after an interruption

Never delete the cache while recovery is pending. Use **Recover** on the original connection and keep its original account and destination available. To renew credentials, use **Reconfigure account**, preserving the server, username, folder and access mode. Destination changes and removal remain blocked until recovery finishes.

Keep the profile and cache together with their binding information. The default profile is `%LOCALAPPDATA%\SDrive`; it holds preferences, encrypted credentials, keys and recovery information. The cache initially lives there and can be changed through Preferences after clean disconnection. SDrive uses its own configuration rather than modifying your regular `rclone.conf`.

Credential protection depends on your Windows user. Copying the profile to another account is not a portable credential backup; use credential-free configuration export and authenticate again instead. See [security and recovery limits](security.md) before using sensitive data.

## Uninstall

Close open files, finish uploads and choose **Exit safely** before uninstalling. Uninstallation preserves your profile, credentials and cache to avoid losing pending uploads, and keeps shared Windows components installed. Only after resolving all pending files should you consider removing the profile manually. Preserve any separately configured cache until recovery is complete as well.
