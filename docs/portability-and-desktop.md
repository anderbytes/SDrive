# Configuration portability and desktop preferences

## Windows startup after reinstalling

The local `settings.json` remains the source of truth for **Start with Windows**. Uninstalling removes the Windows startup entry while preserving the profile. Both installers restore that preference before finishing, even when you do not launch the application. Opening SDrive also reconciles a missing entry or an outdated executable path. Disable startup in SDrive Preferences to keep it disabled; external removal of its Run entry is repaired on the next launch. Separate Windows startup-disable controls are left untouched.

The installer runs the installed executable with `--restore-startup`. This command locks the profile and only reconciles startup: it does not launch the interface, start engines, mount drives, access credentials or rewrite settings. A missing profile is a no-op. Invalid or incompatible settings and pending recovery prevent reconciliation. PIN protection does not prevent restoring this saved integration and remains enforced when the application opens.

If restoration fails, installation offers Retry / Cancel; silent installation returns exit code 1. Installed files and the preserved profile remain available. On application launch, a reconciliation failure leaves the app usable and displays the startup warning; save Preferences to retry after resolving the cause.

Run `./scripts/test-startup-installer.ps1` for an isolated NSIS check of the production restoration block, including helper arguments, success and silent failures. Core startup tests exercise preserved profiles and simulated registry state across reinstallations without touching the user's startup entry.

## Export and import

In **Preferences → Configuration → Portable configuration**, export a local JSON file or review an import. This is a portable definition file, not a restore operation for `settings.backup.json`. It never transfers pending files or their original recovery identity.

The `sdrive-portable` format, version 2 (with version 1 accepted and migrated on import), contains connection names, providers, relative remote paths, mount options, transferable preferences and an explicit allowlist of non-secret access hints. Names, server addresses, usernames and paths may still be private: store and share the file deliberately.

Exports exclude passwords, tokens, OAuth Client IDs/Secrets, access-key IDs and secrets, SAS URLs, private-key/service-account file paths and contents, DPAPI keys, encrypted configuration, proxy configuration, cache files and recovery bindings. Addresses containing credentials, query parameters or fragments cannot be exported. If saved access details are unavailable, export fails rather than claiming to have exported a complete definition. Reauthenticate and retry. Selected SSH keys and service-account files must be supplied again on the destination computer.

Import reads at most 4 MiB and accepts at most 1,000 definitions. Unsupported versions, unknown fields, unsafe values and unsupported services are rejected. Review the selected definitions, edit conflicting names and mount destinations, or deselect entries. The backend checks conflicts and local destination availability again before saving the entire batch. A stale preview or failed save does not partially import the batch.

Imports become persistent **drafts**, separate from operational connections. They do not start a worker, reserve drive letters, auto-connect or participate in recovery/cache cleanup. **Authenticate** opens the existing setup wizard with non-secret hints filled in. Supply application credentials, authorize the provider again, review the remote folder and explicitly connect. HTTP/plain FTP consent and SSH host verification must be repeated. Cancelling or failing setup preserves the draft; successful confirmation creates a fresh connection identity and removes that draft atomically. **Remove draft** only removes the definition.

Applying imported preferences is a separate, initially unchecked choice. It covers theme, language, cache size/timing and desktop preferences. Windows startup, proxy settings and authoritative cache location stay local. Existing mounted drives are not remounted for imported cache settings. Preferences and definitions cannot be imported during protected settings recovery or active setup.

Settings writes now use version 6. Unversioned settings and versions 1–5 remain readable. Older applications reject version 6; do not edit version numbers to bypass this protection.

## Explorer and connection options

**Open Explorer after connecting** is a per-drive option, available in creation and editing, and defaults off for new drives. Existing global preferences migrate to each saved drive. When enabled it opens the mount once after a successful connection, including tray and automatic connections. Repeated status snapshots and failed connects do not open windows. A desktop launch failure does not undo the connected drive or its recovery state.

**Show service icons for drive letters** also defaults off. SDrive supplies original, colored service monograms (for example GD, OD and WD), not third-party logos. Generated ICO files remain in the local profile's `icons` directory; they contain no account information. Folder mounts retain their existing icons.

Icon integration uses only the current user's `Software\Classes\Applications\Explorer.exe\Drives` registry branch. Ownership records bind each value to the profile, executable and expected icon. Existing customization wins. Disconnect, disabling the preference, reconciliation and uninstall remove only unchanged values owned by SDrive; later user edits are preserved. Empty owned keys are removed. Interrupted runs are reconciled on reopening. Icon files may remain in the profile after uninstall, alongside the deliberately preserved profile; no shell override remains pointing to them. Moving the executable to a different installation path does not authorize cleanup of the previous installation's values.

Connected network drives display their configured connection title and drive letter in Explorer, without the default `\\server` suffix. This works with service icons disabled. SDrive resolves the actual network path and sets its per-user `MountPoints2\_LabelFromReg` label, preserving existing user labels. Disconnect, reconciliation and uninstall remove only unchanged labels owned by the same installation and profile (or all its profiles during uninstall). Windows' mount-point keys and unrelated metadata remain intact. Local disks use their volume title; folder mounts are unaffected.

Native Windows Shell resolution has been checked with disposable network-letter and local-disk-letter mounts. Icons and their removal were resolved through the Shell's own APIs. This is not a guarantee of immediate refresh in every third-party file manager.

Connection cards show the provider, remote path and upload/recovery state to the right of the drive letter. **… → Drive details → Connection options** lists read-only access, automatic mounting, session-only credentials, mount presentation, requested proxy routing and directional bandwidth limits. These describe configured options; upload, error and recovery messages remain separate. In particular, **Proxy requested** does not certify a reachable proxy. The same details panel exposes remote destination, uploads, local cache, remote capacity, transport security and alerts in keyboard-accessible vertical sections, with a collapsible menu on narrow windows.

## Focused verification

Core tests cover portable round trips, provider-wide credential exclusion, malformed/future formats, conflicts, failed import and authentication commits, retained drafts and pending recovery. Desktop tests inject callbacks to verify repeated snapshots and launch failures. Platform tests use a separate registry test branch for ownership and cleanup.

`TestWindowsServiceIconsResolveForMountedDriveLetters`, enabled with `SDRIVE_TEST_MOUNT=1`, uses only free letters without existing icon overrides, loopback storage and temporary profiles. It compares rendered native Shell icons against the generated service icon in network/local modes and verifies removal when disabled. No existing user drive is modified.

`TestNetworkDriveDisplaysOnlyConnectionTitleInExplorer`, also enabled with `SDRIVE_TEST_MOUNT=1`, mounts disposable loopback storage with service icons disabled, reads the actual Explorer display name and checks label cleanup after disconnect. Registry unit tests cover title updates, repeated reconciliation, profile/installation ownership and preservation of user edits.

The Wails smoke test also exercises portable export/import, explicit reauthentication and cancellation, draft removal, desktop preference defaults, English/Portuguese labels. Only its dedicated smoke binary substitutes a deterministic file-picker path inside the disposable test workspace; production builds use native file dialogs.

## Appearance preferences

Preferences separates General, Appearance, Cache, Network and Configuration settings. Appearance offers System, Light and Dark themes. System is the default for new and older profiles and follows Windows theme changes. Selecting a theme previews it immediately; Save persists it, while Cancel or Escape restores the saved choice. Failed saves leave the preview open for correction or cancellation. The saved theme also applies to the application lock screen.

Portable exports include the theme. Importing it requires selecting the imported preferences checkbox; older portable files without a theme use System when those preferences are applied. Cache-directory changes and import/export actions take effect independently of Save and are not undone by Cancel.

## Restore cache and proxy defaults

In **Preferences → Cache**, restore the size target (**2 GiB**) or timing (**24 hours** retention, **5 seconds** upload delay) separately. These actions edit only the selected form values. **Save** persists them for the next connection; **Cancel** discards unsaved resets. Other preferences and connections are preserved.

**Restore default cache location** is an explicit, immediate action independent of Save/Cancel. It resolves the default from the current profile, requires disconnected drives, finished setup/operations and verified clean cache, and preserves the old directory. It never migrates or deletes files. Only that exact profile-local destination is exempt from the usual outside-profile restriction. An existing default may contain only its valid ownership marker and empty directories; files, unknown entries, links or insufficient disk space block restoration. A failed switch leaves the previous binding authoritative. Restoring an already-active default is a no-op.

In **Preferences → Network**, **Restore proxy defaults** clears the form's address, username and password. Save removes the saved credentials; Cancel leaves them intact. Per-connection proxy choices remain set, so those connections fail with a configuration message until a proxy is configured again. Already-connected workers retain their next-connection semantics; restoring does not disconnect drives or silently bypass a required proxy.

New controls and safety messages are supplied in English and Brazilian Portuguese. Other interface locales currently use the English fallback for these additions.

## Drive card appearance

Drive cards show a status tag beside the title, followed by a colored connection-type tag and the server/folder destination. Local mount folders, experimental status and warnings appear below these primary details. Folder mounts use a folder icon instead of a drive letter.

Choose **… → Customize appearance** to select Default, Blue, Violet, Teal or Amber and optionally replace the letter/folder icon with a local PNG, JPEG or WebP image (up to 5 MiB). SDrive stores a normalized PNG copy up to 128 × 128 pixels and 128 KiB, so moving the original image has no effect. Remove image restores the generated icon. Save persists the preview; Cancel and Escape discard it. These choices are included in portable configurations and imported drafts; older configurations use the default appearance.

Appearance can be changed while mounted or awaiting recovery, without restarting the connection or changing credentials, destination or pending files. Active operations, setup for that connection, application lock and invalid settings block the save. Custom card backgrounds follow the application theme; high contrast retains system colors.

Server summaries are read from local configuration, cached per destination and sanitized before reaching the interface. They omit URL credentials, query parameters and fragments. Providers without an available endpoint show only the remote folder or Root. Reading a summary does not contact the storage server; an existing per-drive configuration reader may be started to access its encrypted local settings.
