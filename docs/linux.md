# Linux desktop support

SDrive targets Ubuntu 24.04 or later and Debian 13 or later on x64, with a graphical X11 or Wayland session. It uses GTK4, WebKitGTK 6.0, a desktop Secret Service credential store and FUSE3. It runs as your regular user; do not launch SDrive with sudo.

## Install and open

Install the `.deb` using your distribution's graphical package installer, or run `sudo apt install ./SDrive-<version>-linux-amd64.deb`. Install and unlock a compatible credential store such as GNOME Keyring or a KWallet configuration that exposes Secret Service. The package recommends GNOME Keyring. Start **SDrive** from the applications menu.

The portable `.tar.gz` contains SDrive, its pinned rclone engine, licenses and this guide. Extract both executables together and run `./SDrive`. Portable describes the installation layout: application data still belongs to your local user profile, and system libraries are required. On supported distributions, install the runtime libraries with:

```sh
sudo apt install libgtk-4-1 libwebkitgtk-6.0-4 fuse3 xdg-utils dbus-user-session gnome-keyring
```

Create a connection and select an existing parent directory with a new subfolder name. SDrive creates and owns that empty folder for FUSE. Existing folders or files are never overwritten to prepare a mount. After a confirmed clean disconnect, only the empty folder created by SDrive is removed. A leftover owned empty folder can be reused after an interruption; changed, occupied or unverifiable folders require attention.

The user must be able to access `/dev/fuse`, and `fusermount3` must be installed. Containers need explicit FUSE device access. SDrive does not install kernel components, enable `allow_other`, modify AppArmor rules or request administrator access to mount files.

## Desktop integration and security

- Open launches the default file manager. OAuth uses the default browser or an explicitly selected installed browser, including its supported private mode.
- Start when signing in creates a user-owned XDG autostart entry. It launches in the tray when the desktop provides StatusNotifier support; otherwise the window stays accessible.
- Closing the main window keeps drives running. Without a usable tray, closing minimizes the window. Always use **Exit safely** to finish uploads and stop workers.
- The profile lives at `$XDG_DATA_HOME/SDrive`, normally `~/.local/share/SDrive`. Its persistent cache defaults to the `cache` subdirectory. Directories are private to the owning user, and private files use mode `0600`.
- Configurations, proxy credentials and the PIN record are encrypted. Random keyring lookup references contain no keys. A locked or missing keyring blocks credential use; SDrive never falls back to plaintext or silently replaces a lost key.
- Manual PIN protection controls SDrive management, while mounted files remain accessible to applications. Global inactivity automation, Windows drive letters/icons, Windows hidden-file attributes and the Windows file-process inspector are unavailable on Linux. The file manager controls dotfile visibility.

Use portable configuration export/import to move between Windows and Linux. Choose a valid mount destination on the receiving system and authenticate each imported draft again. Copying a DPAPI-protected Windows profile does not transfer credentials or pending uploads. Keep the original profile and cache until recovery has completed there.

After a hard interruption, FUSE may leave an unavailable mount reporting “Transport endpoint is not connected”. SDrive preserves it instead of detaching an unverifiable mount automatically. Close applications using that folder, confirm that the old connection worker has stopped, and run `fusermount3 -u -- /absolute/path/to/your/mount`. Keep the profile and cache intact, then reconnect the same connection to recover pending uploads. Do not remove cache files or replace credentials to clear this condition.

Exit safely before replacing, upgrading or removing SDrive. The `.deb` refuses changes while the application is running. Removing the package preserves local profiles, keyring entries and pending files. For portable installations, close SDrive before replacing either executable.

## Build and validation

Install Go 1.26.10, Node.js 24 or later and the build dependencies:

```sh
sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev dpkg-dev zip
bash scripts/build-linux.sh
```

Run the build inside an unlocked Secret Service desktop session. It builds the frontend and application, verifies pinned rclone and SMB module checksums, applies the SMB security and unprivileged network-accounting extensions, runs its tests, and produces packages, corresponding sources, licenses and `SHA256SUMS` in `bin/linux-amd64`. The portable package depends on system GTK/WebKit libraries; it is not a self-contained AppImage. Builds made on a newer distribution may require newer system libraries; the Ubuntu 24.04 CI artifact is the baseline distribution.

For native validation install `dbus-x11 gnome-keyring xvfb xauth weston`, then run `bash scripts/build-linux.sh --check` from an unlocked D-Bus session. It requires writable `/dev/fuse`; missing native prerequisites fail the check. The test runner includes the platform and engine suites, focused Linux core tests, real local WebDAV/FUSE lifecycle tests, and native WebKit bridge/rendering checks under X11 and headless Wayland. Windows-specific suites remain on Windows CI. CI also checks `.deb` installation and removal.

Automated fixtures use synthetic local files and services. They do not certify real provider accounts, every desktop environment, interactive OAuth completion, or file-manager behavior. A successful cross-compilation alone does not establish native runtime support. Consult CI results and the contributor validation matrix before treating a build as verified for your environment.
