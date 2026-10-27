<p align="center">
  <img src="assets/sdrive.png" alt="SDrive cloud and drive logo" width="88">
</p>

# SDrive

**Your cloud files, right in Explorer.**

SDrive brings your cloud storage and remote servers into Windows File Explorer. Open, edit and save files with the applications you already use, without downloading everything first.

Free and open source, for **Windows 11 x64**.

[Features](#what-you-can-do) · [Get started](#get-started) · [User guide](docs/usage.md) · [Contribute](#help-improve-sdrive)

![SDrive showing a connected Documents drive and a disconnected Team files drive](assets/screenshots/overview.png)

*The actual interface with fictional demonstration data. This illustration does not represent testing against live services.*

## What you can do

- **Work from Explorer.** Connect storage as a drive letter or a local folder and use your usual applications.
- **Keep connections together.** Open, connect and disconnect your drives from one window or the system tray.
- **See what is happening.** Check connection status, storage space when available, pending uploads and recovery alerts.
- **Make it yours.** Choose light, dark or system appearance, change the interface language and personalize drive cards.
- **Choose how you connect.** Set read-only access, automatic connections and transfer limits when needed.
- **Reuse your setup.** Export connection definitions without credentials and sign in again after importing them on another computer.

Connection options include Google Drive, OneDrive, Dropbox, Nextcloud, SFTP, WebDAV, SMB and S3-compatible storage, among others. Availability and behavior depend on the service and account. See the [connection validation matrix](docs/provider-validation.md) for coverage and limitations.

## Get started

1. Open this repository's **Releases** page and download the recommended **online installer**. It downloads missing components for you. Choose the **offline installer** if installation must work without internet access or your network restricts downloads.
2. Run the installer and follow its instructions. Allow Windows to prepare virtual drives if prompted; restart only if requested.
3. Open SDrive, choose **New connection** and select your service.
4. Sign in or enter the access details for your storage, choose a folder and select **Connect drive**. Use **Open in Explorer** to browse your files.

Both installers need network access to sign in and use remote storage. Some sign-in methods require registering your own application with the provider; the [sign-in setup guide](docs/oauth-setup.md) explains these additional steps.

Need more detail? The [user guide](docs/usage.md) covers everyday use, preferences, updates and troubleshooting. You do not need to build SDrive or run scripts to install it.

## Keep your files safe

Saving a file to a connected drive does **not** mean its upload has finished. Close open files and wait for uploads before disconnecting or choosing **Exit safely**. Closing the window keeps SDrive running in the tray.

If recovery is pending, keep the cache and connection data intact and follow the [recovery instructions](docs/usage.md#recover-after-an-interruption). SDrive accesses files on demand; it is not a full offline synchronization or backup tool.

## Help improve SDrive

As the maintainer, I cannot personally test every connection type, feature, account configuration or Windows environment. SDrive relies on the community's understanding and support to improve compatibility and reliability.

You can help by reporting reproducible problems, testing with disposable data, improving translations or contributing code and documentation. Implemented features and automated tests do not guarantee that every real-world scenario has been verified. Please share only sanitized diagnostics and screenshots, never credentials or private files.

See [how to contribute](docs/CONTRIBUTING.md) and the [provider testing checklist](docs/provider-validation.md#checklist-for-each-contribution). Reports of what works, what fails and what remains untested are all useful.

## Learn more

- [User guide](docs/usage.md): installation, preferences, updates, diagnostics and recovery.
- [Security and privacy](docs/security.md): credential protection, local diagnostics and their limits. SDrive sends no telemetry.
- [Connection options](docs/connectivity.md) and [sign-in setup](docs/oauth-setup.md).
- [Development](docs/development.md) and [translations](docs/localization.md).

## License

SDrive is licensed under [GPL-3.0-only](LICENSE). Included components retain their own licenses; see [third-party notices](THIRD_PARTY_NOTICES.md).
