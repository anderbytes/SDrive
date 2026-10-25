# SMB transport security extension

These additions target rclone v1.75.2 and its pinned go-smb2 dependency. They expose the existing session/share encryption flags through the read-only `transport-security` backend command. They do not change protocol negotiation, require encryption, disable signing or write remote files.

The source files use `.go.in` so the application module does not compile upstream package additions as application packages. `scripts/engine-overlay.ps1` maps them to `.go` files in private copies of the verified modules, outside GOMODCACHE. It uses a private module file to replace go-smb2 with that copy. Upstream module files remain unchanged.

From the root of an extracted `SDrive-source.zip`, run `scripts/bootstrap.ps1` to build the modified engine. Run `scripts/engine-overlay.ps1 -Test` for the focused extension tests. The root source archive includes these additions in their original layout; `rclone-source.zip` contains the unchanged upstream rclone sources. The distribution also includes the pinned dependency source archive and its license. The engine build stamp records both the upstream checksum and the digest of these additions.

The command samples a live pooled connection to the selected share. Encryption is active when either the negotiated session or share requires encryption. Signing alone is not encryption. Root mounts without a specific share return an error. The SDrive backend reports unknown on failed or unsupported queries and invalidates samples when the connection lifecycle changes.

The source additions follow the licenses of the packages they extend: rclone's MIT license and go-smb2's BSD license. Original notices are retained in the corresponding upstream sources and distributed licenses.
