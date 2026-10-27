# Cache, mount destinations and Explorer browsing

## Cache location and timing

Preferences shows the authoritative cache directory and accepts an empty, dedicated directory on an available local disk. It must be outside the profile, with at least 5 GiB free, and must not overlap a mount destination. Network paths, linked ancestors and occupied directories are rejected.

Disconnect every drive and finish setup and recovery first. SDrive checks the actual mount inventory, stops isolated workers and freshly inspects durable metadata. Pending, corrupt, unpaired or unrecognized cached data blocks the change. Display samples do not authorize it.

The transition activates a new empty cache without moving or deleting the previous cache. Failures before the location commit leave the previous directory authoritative. Preparing the new directory can leave an ownership marker after a failed commit; preserve it and choose another empty directory when retrying. Neither directory is automatically cleaned up.

The profile's `cache-location.json` binds the path, an ownership token and the directory's Windows identity. The cache contains `.sdrive-cache-owner`. Keep these files with the profile and cache. Settings restoration preserves this binding. An unavailable disk, missing metadata, replaced directory or ownership mismatch blocks mounting and cleanup instead of silently starting an empty cache elsewhere. Restore the original disk/directory and restart SDrive; do not move the cache manually.

Settings writes use version 5; unversioned, version 1, version 2, version 3 and version 4 settings remain readable with their original defaults. Both settings generations are upgraded before activating an external cache, preventing older applications from ignoring its binding. Do not edit version numbers to bypass compatibility protection.

| Preference | Default | Meaning |
| --- | --- | --- |
| Cache target | 2 GiB per connection | Desired size, not guaranteed disk consumption |
| Retention | 24 hours | Inactivity since last access before content becomes eligible for eviction |
| Upload delay | 5 seconds | Delay after last use before write-back; zero requests immediate scheduling |

Retention accepts 1–4320 hours and upload delay accepts 0–3600 seconds. Changes apply on the next connection. Open files, pending uploads and periodic eviction can exceed size/age targets. Saving a file locally never confirms its remote upload. There is no cache-delete action.

## Remote changes and browsing traffic

Each connection has a directory metadata lifetime, default 300 seconds, range 1–86400 seconds. External additions, renames and deletions appear on directory access after expiry. Supported backends may detect changes sooner: the 60-second backend polling interval is retained only when it is shorter than the directory lifetime. This is separate from SDrive status polling and cache inspection.

**Refresh files in Explorer** explicitly refreshes the mounted remote tree through `vfs/refresh`. Reopen the Explorer folder afterwards. This refresh merges directory listings with pending local writes; it does not clear cached content. Large trees can require many requests. The application request has a 15-second deadline; timeout does not prove the engine stopped processing it. Status polling never triggers recursive refresh.

The pinned engine already reads content on demand. Isolated WebDAV checks distinguish directory/property requests from content reads. Explorer or another application can still open files for thumbnails, indexing or content-based properties. Such reads can download data. SDrive has no reliable per-drive thumbnail suppression switch and does not alter global Explorer preferences.

Modification dates and checksum behavior remain unchanged. `NoModTime` also stops preserving modification dates, so it is not exposed as a harmless request-reduction switch. Network-drive presentation remains the default.

## Remote capacity

Cards show provider-reported total, used and available space separately from local cache usage and cache-disk free space. Capacity may cover the account rather than the chosen folder. Missing fields remain unknown; zero requires an explicit provider value. Unsupported and unavailable states do not block mounting.

Queries use already-active workers, with one-minute sampling and the snapshot deadline. They do not start disconnected providers or scan remote files to invent a quota.

## Drive letters and folders

The default remains a network drive with a letter between D and Z. Local-disk presentation is optional. Both run in the current Windows session without a service or elevation changes; ordinary and elevated applications can see different drive letters. Folder paths remain subject to their parent's Windows visibility and permissions.

Folder mounting requires local-disk mode and a new, nonexistent subfolder of an existing local directory. Select the parent and edit the suggested subfolder name. Even existing empty folders are rejected because the pinned Windows mount requires the final path not to exist. SDrive never removes a directory or hides existing contents to make mounting succeed.

Validation rejects device/network paths, ambiguous names, linked ancestors and overlap with the profile, cache or another saved mount. It runs again before mounting. Destination changes remain blocked during mounting, operations or pending recovery. Folder connections do not reserve letters.

Writable local mounts use the fixed WinFsp `FileSecurity=D:P(A;;FA;;;OW)` owner ACL and restrictive VFS permissions. POSIX permissions alone would retain some metadata permissions for other users. This does not protect against administrators or programs running as your user.

Read-only connections use `FileSecurity=D:P(A;;FRFX;;;OW)` for all mount destinations, allowing the owner to read and traverse without write or delete access, with VFS permissions `0400` for files and `0500` for directories. The VFS read-only guard remains enabled. Windows rejects deletion, renaming and writes immediately, so Explorer does not appear to delete a file that returns after refresh. Rejecting deletion only in the VFS is too late: WinFsp can call unlink during cleanup, where the error does not reach the caller. These permissions apply on the next mount; disconnect and reconnect an existing mount to apply an updated access mode or this fix.

Explorer, tray actions, reconciliation and safe disconnect use the saved destination. The engine removes its virtual folder mountpoint on disconnect; SDrive does not delete an independently created replacement directory.

## Validation and engine references

Native checks use temporary profiles, loopback WebDAV, disposable files and existing WinFsp/rclone components. They cover both letter presentations and folders, alternate cache, external changes, pending writes, open-file disconnect, collisions, reopening and process interruption/recovery. They do not certify cloud accounts, physical power loss or a second interactive Windows account.

Recheck the pinned rclone 1.75.2 [VFS options](https://github.com/rclone/rclone/blob/v1.75.2/vfs/vfscommon/options.go), [directory/dirty-entry handling](https://github.com/rclone/rclone/blob/v1.75.2/vfs/dir.go), [RC refresh](https://github.com/rclone/rclone/blob/v1.75.2/vfs/rc.go) and [Windows mount/permission rules](https://github.com/rclone/rclone/blob/v1.75.2/cmd/mountlib/mount.md) on upgrades.

## Document representation and virtual capacity

Google Drive setup and the Access section of Edit drive offer preferred download formats in Advanced options: Office (the existing DOCX/XLSX/PPTX/SVG default), PDF with Office/SVG fallbacks, or OpenDocument/SVG. The engine chooses the first supported format for each native document. Exported extensions and bytes follow that selection; uploading files does not convert them to Google documents. Existing binary files are unchanged. A format change requires a disconnected drive and a fresh clean-cache check. Pending recovery only permits the original format; nondefault formats participate in the destination hash and session-only binding. Reconfiguration creates a separate remote/cache identity. Portable definitions retain the selection without credentials.

Virtual capacity in Explorer is an optional whole GiB value in the connection's file options (0 uses the provider default; maximum 8,388,607 GiB). It maps to the pinned engine's `DiskSpaceTotalSize` VFS option, applies after reconnecting and survives local and portable configuration. It changes the volume's reported total only: it neither reserves space nor enforces a quota. Free/used values still follow engine/provider reporting; SDrive's provider capacity and local cache free space remain separate. Invalid, fractional and overflowing inputs are rejected.

Engine references: [Google document export formats](https://rclone.org/drive/#drive-export-formats) and [VFS disk options](https://rclone.org/commands/rclone_mount/#vfs-disk-options). Behavior was also checked against the pinned engine source.

## Hidden files and card appearance

Hidden file visibility is configured per drive: **Follow file manager settings** (default for new drives), **Always show**, or **Always hide**. These modes apply to file and folder names beginning with a dot at every depth. On Windows, following the file manager uses WinFsp's `dothidden` attribute; showing omits that attribute; hiding applies an isolated rclone mount-listing exclusion. The application never changes Explorer's global settings or deletes hidden files. On Linux, showing dot-prefixed names remains a file-manager choice; following and forced hiding are available. Changes apply on the next connection, survive export/import, and cannot change during pending recovery. Provider-specific hidden attributes on other names are outside this setting. Legacy checked or absent checkboxes migrate to Always show; unchecked checkboxes migrate to Follow file manager settings.

Opening Explorer after connection is also configured per drive, and defaults to off for new drives. Existing global choices migrate to each saved drive. It runs once after a successful connection, including automatic connections, and never from status refreshes.

Drive order can be changed by dragging its handle or by focusing the handle, pressing Space, moving with arrow keys, and pressing Space to save (Escape cancels). The order persists; filtered moves keep hidden drives in their existing slots. Filters combine search, connection state, provider, mount presentation and read/write access, and last only for the current application session. Imported authentication drafts stay visible separately.

The Eye **Details** panel provides public connection settings without disconnecting. Other status icons keep their specialized information. **Drive diagnostics** is in the card menu immediately above removal. The general diagnostic report opens in its own modal from **About SDrive**, where users can refresh, copy or export the exact reviewed text as a local UTF-8 file. No report is sent automatically.

The About dialog shows the release date beside the application version. `frontend/package.json` owns the fixed `releaseDate` (ISO calendar date); recompiling the same release does not change it. Local settings use format 6 and portable exports use format 2; previous supported local formats and portable format 1 migrate on read. Older SDrive releases cannot read these new formats.

Preferences → Appearance chooses whether the left card stripe follows the theme or the drive's existing custom color. The default is the theme color, which also applies to drives without a custom color. Connection status remains in the card's status indicators. This preference survives export/import.
