# Connection authentication and networking

New-drive setup starts in **Basic**. Password/address access, S3, B2, Azure and GCS service accounts belong there. **Advanced** contains the OAuth methods that require your own application registration; see [OAuth setup](oauth-setup.md). These tabs are separate from the access form's Advanced options.

## Isolated connection engines

Each connection uses its own supervised rclone process and encrypted configuration under `connections/sd_<remote>/`. It starts on demand; processes remain available after setup and stop after a clean disconnect. The root encrypted configuration remains the legacy source for existing profiles. First use copies and verifies a legacy remote without overwriting that source. Configurations are never written concurrently by multiple processes. Remote names and the common cache location are unchanged.

Settings writes use version 5; the application reads versions 1, 2, 3 and 4 and the unversioned format. Older SDrive versions reject version 5 instead of ignoring new safety fields. The [cache-location binding](mount-runtime.md) is preserved independently of settings restoration. Preserve the complete profile, including root and connection-specific keys/configuration and cache, during recovery. Isolating a legacy remote does not authorize deleting its cache or changing its destination.

## Bandwidth and proxy

Upload/download limits are independent per connection, expressed in KiB/s in the interface, with zero meaning unlimited. They limit the corresponding process, not all SDrive drives together. The pinned engine's directional limiter applies at the HTTP transport; SDrive enables it for Drive, OneDrive/SharePoint, Dropbox, Box, pCloud, WebDAV/Nextcloud/ownCloud, S3, B2, Azure Blob and GCS. FTP, SFTP, SMB, MEGA and Proton Drive do not expose this guarantee in this delivery. Short bursts, protocol overhead and local cache reads mean an instantaneous Explorer rate is not the configured average remote-transfer ceiling.

Configure an HTTP, HTTPS or SOCKS5 proxy in Preferences, with username/password separately, then opt each supported HTTP connection into it. The same allowlist applies to proxy routing. Proxy secrets are encrypted locally with a key protected by DPAPI, never returned in snapshots or diagnostics, and not included in process arguments. Leaving the password blank preserves it only for the same proxy address and username. Clearing the address disables that proxy configuration; a connection still opting in will report the missing configuration.

Both `socks5://` and `socks5h://` use proxy-side destination hostname resolution. SOCKS5 authentication is optional; when enabled, provide both username and password, each at most 255 UTF-8 bytes. The proxy hostname itself still needs local resolution unless it is an IP address. SOCKS5 does not encrypt the client-to-proxy connection; retain HTTPS for remote storage and use a trusted proxy or secured tunnel. Storage requests, worker OAuth token requests using the shared HTTP transport, and SDrive WebDAV destination verification follow the configured proxy. Browser authorization uses the external browser's own network settings. A failed proxy never causes these configured transports to retry directly. This option does not add proxy routing to FTP, SFTP or SMB.

Bandwidth, proxy and timeout changes apply when the next connection process starts. Existing configured processes keep their effective settings until disconnected. Inherited proxy/rclone environment overrides are removed from child processes. SDrive's authenticated loopback RPC remains direct even when a provider proxy fails. Provider-specific networking and consent still require real-account acceptance; the local tests use a disposable WebDAV server and rejecting HTTP proxy.

## Session-only sign-in

This explicit mode stores no reusable connection credentials in persistent configuration. Its worker uses rclone's in-memory configuration. Disconnecting, cancelling unsaved setup, closing a saved-but-unmounted setup, or exiting discards its credentials. Connect opens authentication again, and automatic mounting is unavailable. OAuth may reuse browser cookies; this mode does not promise a fresh password prompt from the provider. Proxy credentials are a separately configured global preference.

The mode is available only where a destination can be bound reliably: OneDrive/SharePoint library IDs, Google Drive with an explicit root/shared-drive ID, and HTTPS WebDAV exposing `DAV:resource-id`. Dropbox, Box, pCloud, GCS, password protocols and other unverified combinations cannot enable it. A default Google personal root is insufficient. Verification failures stop confirmation and preserve the previous profile.

Only non-secret destination binding, connection preferences and recovery identifiers persist. After an interruption, use the saved connection to authenticate again, keeping its remote/cache namespace, original library/root or WebDAV resource identity and folder. A different identity cannot receive pending writes. Failed/unknown cache checks still block safe exit. Authentication cannot recreate a deleted remote destination or restore access revoked by its owner; preserve the profile and pending files when access cannot be restored.

Changing an existing persistent connection into session-only mode requires clean disconnect, no pending recovery and explicit confirmation to remove its saved credentials. SDrive removes that connection's credential entries from the legacy and isolated files before enabling the mode. A failure prevents activation; the user may need to authenticate again if cleanup succeeded but a later settings write failed. This is logical removal from SDrive-managed storage, not forensic erasure from disks, OS memory/pagefiles or external backups.

## Verified alternate WebDAV address

For HTTPS WebDAV, Nextcloud and ownCloud, setup can verify an alternate address using read-only depth-zero PROPFIND requests for the selected folder. Both addresses must return the same globally unique `DAV:resource-id` as defined by [RFC 5842](https://www.rfc-editor.org/rfc/rfc5842.html). Servers that omit that property cannot enable fallback. Equality of names, directory contents, DNS addresses or a user checkbox is insufficient. No test marker is written.

Only a primary connection timeout/unreachable address during Connect can select the alternative. The alternative must still return the saved resource identity. Authentication, permission, certificate/protocol errors and changed/missing identities stop the connection. Requests never follow redirects with credentials. A healthy primary does not depend on the alternative being online.

SDrive does not switch endpoints under an active mounted filesystem. If the network fails while mounted, pending data remain preserved; recover using the saved connection after the interrupted mount is gone, or wait for the primary. The canonical URL/destination hash and remote cache name remain stable even if an interrupted worker had selected the alternative. Changing addresses or the selected verified folder requires Reconfigure account and cannot bypass pending recovery.

## Protocol controls and unsupported charset conversion

WebDAV exposes connection timeout and HTTP I/O idle timeout in seconds. Zero retains rclone defaults (one minute to connect and five minutes idle in the pinned engine); this is not a total duration limit for a whole transfer. FTP exposes disabling EPSV in favor of PASV and disabling MLSD in favor of LIST for compatibility. Explicit/implicit FTPS and certificate verification retain their existing behavior.

FTP is passive-only. Active mode, Mode Z compression and legacy charset conversion (for example Windows-1252) are not supported by the pinned engine. `encoding` controls filename escaping and is not charset transcoding; disabling UTF-8 negotiation would not implement lossless conversion. Unicode behavior is unchanged. Unicode/content checks do not certify every older server's filename behavior.
