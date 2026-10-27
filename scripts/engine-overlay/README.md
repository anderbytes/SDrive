# Engine transport extensions

These additions target rclone v1.75.2 and its pinned go-smb2 dependency. They expose the existing session/share encryption flags through the read-only `transport-security` backend command. They do not change protocol negotiation, require encryption, disable signing or write remote files.

The source files use `.go.in` so the application module does not compile upstream package additions as application packages. `scripts/engine-overlay.ps1` maps them to `.go` files in private copies of the verified modules, outside GOMODCACHE. It uses a private module file to replace go-smb2 with that copy. Upstream module files remain unchanged.

From the root of an extracted `SDrive-source.zip`, run `scripts/bootstrap.ps1` to build the modified engine. Run `scripts/engine-overlay.ps1 -Test` for the focused extension tests. The root source archive includes these additions in their original layout; `rclone-source.zip` contains the unchanged upstream rclone sources. The distribution also includes the pinned dependency source archive and its license. The engine build stamp records both the upstream checksum and the digest of these additions.

The command samples a live pooled connection to the selected share. Encryption is active when either the negotiated session or share requires encryption. Signing alone is not encryption. Root mounts without a specific share return an error. The SDrive backend reports unknown on failed or unsupported queries and invalidates samples when the connection lifecycle changes.

The source additions follow the licenses of the packages they extend: rclone's MIT license and go-smb2's BSD license. Original notices are retained in the corresponding upstream sources and distributed licenses.

## Network accounting

The authenticated `sdrive/network` RC endpoint accepts `start`, `stats` and `stop` actions with a nonempty session identifier. Starting the same identifier is idempotent; stats/stop reject a different identifier. Each isolated worker holds one in-memory session. Network samples never store paths, credentials, packet contents or hostnames.

`prepare-network.mjs` verifies unique source anchors before overlaying the pinned common dialer's Read/Write calls and file-accounting hooks. It fails the build when those anchors change. Both build platforms use the same preparation code. Windows and Linux count successful socket bytes, including bytes returned together with an error, below TLS/SSH. HTTP storage, FTP/FTPS, internal SFTP, SMB, MEGA and Proton Drive use that shared transport in the supported configuration. External SSH and override configurations are reported as partial coverage. DNS performed by the resolver, kernel TCP/IP overhead, ACKs, retransmissions, TCP keepalive and VPN encapsulation are outside this counter. The local RC listener is not instrumented.

File-content accounting follows remote-to-local/VFS downloads and local-to-remote uploads, including active transfers; checks, local cache reads and server-side copies do not increment it. Retries may read content again, and application content is not equivalent to wire bytes. Listings, periodic checks, authentication, application retries and encryption overhead contribute to the measured socket total but cannot be separated reliably. The UI must never infer their sizes by subtracting the content counters. Counters are independent of the bounded completed-transfer history and RC statistics groups.

Run the focused network tests through the existing overlay test runners. The real-engine `TestNetworkCountersIncludeMetadataAndIsolateWorkers` fixture verifies metadata-only traffic, worker isolation, control-API exclusion, file directions, stop and reconnect without privileged APIs or a real provider.
