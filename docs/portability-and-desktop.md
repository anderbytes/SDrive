# Configuration portability and desktop preferences

## Export and import

In **Preferences → Portable configuration**, export a local JSON file or review an import. This is a portable definition file, not a restore operation for `settings.backup.json`. It never transfers pending files or their original recovery identity.

The `sdrive-portable` format, version 1, contains connection names, providers, relative remote paths, mount options, transferable preferences and an explicit allowlist of non-secret access hints. Names, server addresses, usernames and paths may still be private: store and share the file deliberately.

Exports exclude passwords, tokens, OAuth Client IDs/Secrets, access-key IDs and secrets, SAS URLs, private-key/service-account file paths and contents, DPAPI keys, encrypted configuration, proxy configuration, cache files and recovery bindings. Addresses containing credentials, query parameters or fragments cannot be exported. If saved access details are unavailable, export fails rather than claiming to have exported a complete definition. Reauthenticate and retry. Selected SSH keys and service-account files must be supplied again on the destination computer.

Import reads at most 4 MiB and accepts at most 1,000 definitions. Unsupported versions, unknown fields, unsafe values and unsupported services are rejected. Review the selected definitions, edit conflicting names and mount destinations, or deselect entries. The backend checks conflicts and local destination availability again before saving the entire batch. A stale preview or failed save does not partially import the batch.

Imports become persistent **drafts**, separate from operational connections. They do not start a worker, reserve drive letters, auto-connect or participate in recovery/cache cleanup. **Authenticate** opens the existing setup wizard with non-secret hints filled in. Supply application credentials, authorize the provider again, review the remote folder and explicitly connect. HTTP/plain FTP consent and SSH host verification must be repeated. Cancelling or failing setup preserves the draft; successful confirmation creates a fresh connection identity and removes that draft atomically. **Remove draft** only removes the definition.

Applying imported preferences is a separate, initially unchecked choice. It covers language, cache size/timing and desktop preferences. Windows startup, proxy settings and authoritative cache location stay local. Existing mounted drives are not remounted for imported cache settings. Preferences and definitions cannot be imported during protected settings recovery or active setup.

Settings writes now use version 4. Unversioned settings and versions 1–3 remain readable. Older applications reject version 4; do not edit version numbers to bypass this protection.

## Explorer and connection options

**Open Explorer after connecting** defaults off. When enabled it opens the mount once after a successful connection, including tray and automatic connections. Repeated status snapshots and failed connects do not open windows. A desktop launch failure does not undo the connected drive or its recovery state.

**Show service icons for drive letters** also defaults off. SDrive supplies original, colored service monograms (for example GD, OD and WD), not third-party logos. Generated ICO files remain in the local profile's `icons` directory; they contain no account information. Folder mounts retain their existing icons.

Integration uses only the current user's `Software\Classes\Applications\Explorer.exe\Drives` registry branch. Ownership records bind each value to the profile, executable and expected icon. Existing customization wins. Disconnect, disabling the preference, reconciliation and uninstall remove only unchanged values owned by SDrive; later user edits are preserved. Empty owned keys are removed. Interrupted runs are reconciled on reopening. Icon files may remain in the profile after uninstall, alongside the deliberately preserved profile; no shell override remains pointing to them. Moving the executable to a different installation path does not authorize cleanup of the previous installation's values.

Native Windows Shell resolution has been checked with disposable network-letter and local-disk-letter mounts. Icons and their removal were resolved through the Shell's own APIs. This is not a guarantee of immediate refresh in every third-party file manager.

Connection cards show accessible text badges for read-only access, automatic mounting, session-only credentials, mount presentation, requested proxy routing and directional bandwidth limits. These describe configured options; upload, error and recovery messages remain separate. In particular, **Proxy requested** does not certify a reachable proxy.

## Focused verification

Core tests cover portable round trips, provider-wide credential exclusion, malformed/future formats, conflicts, failed import and authentication commits, retained drafts and pending recovery. Desktop tests inject callbacks to verify repeated snapshots and launch failures. Platform tests use a separate registry test branch for ownership and cleanup.

`TestWindowsServiceIconsResolveForMountedDriveLetters`, enabled with `SDRIVE_TEST_MOUNT=1`, uses only free letters without existing icon overrides, loopback storage and temporary profiles. It compares rendered native Shell icons against the generated service icon in network/local modes and verifies removal when disabled. No existing user drive is modified.

The Wails smoke test also exercises portable export/import, explicit reauthentication and cancellation, draft removal, desktop preference defaults, English/Portuguese labels. Only its dedicated smoke binary substitutes a deterministic file-picker path inside the disposable test workspace; production builds use native file dialogs.
