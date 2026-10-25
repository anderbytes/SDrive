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

Retention accepts 1–720 hours and upload delay accepts 0–3600 seconds. Changes apply on the next connection. Open files, pending uploads and periodic eviction can exceed size/age targets. Saving a file locally never confirms its remote upload. There is no cache-delete action.

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
