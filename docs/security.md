# Security and limitations

## Trust boundaries

- The interface loads local files and has a CSP. It has no telemetry, CDN or remote interface code.
- The Wails bridge exposes specific operations. The rclone RC API is not exposed directly to the interface; it listens only on loopback, with a random password per run and no open CORS.
- RC API access grants the privileges of the process owner. Do not share the endpoint or its credentials.
- Secrets persist in encrypted rclone configuration. The random key is protected by DPAPI. Only helper process environments receive the required keys.
- DPAPI and directory permissions do not protect against malware running as the same user, administrators, memory capture or Windows compromise.
- The cache contains file content without additional SDrive encryption. Use system disk protection for sensitive data.

## Transport and providers

TLS certificates are verified. HTTP and plain FTP require explicit selection per connection. For SFTP, the public key is obtained without authentication; the user verifies its fingerprint and accepts the key before sending credentials. Key changes are not accepted automatically. This initial verification helps avoid trusting a malicious intermediary.

OAuth opens the external browser. New guided connections use desktop identities maintained by the project, without silently falling back to shared rclone clients. Each provider registration must be ready before advertising sign-in. Existing connections with a custom identity keep that identity when the project has not configured one yet. Provider limits, administrator consent and policies may prevent sign-in; SDrive does not bypass those restrictions. Proton Drive depends on a beta backend.

Embedded desktop OAuth identities are public application metadata, not a way to store server secrets. User tokens and credentials remain in the encrypted file. Known additional steps have English source messages and interface translations; unknown steps stop the wizard with support instructions.

## Writes and recovery

The VFS cache is persistent. A potentially interrupted session is recorded before mounting. Pending files are checked through the rclone API and the local metadata `Dirty` field, including open files that have not entered the upload queue. Failure to read this metadata prevents declaring an exit safe.

Before disconnecting, close files in other applications. There is no distributed transaction between an editor, Windows, cache and provider. Forced shutdown, disk failure or data an editor has not submitted to the system can cause losses SDrive cannot recover.

After an interruption, keep the cache and original destination. Changing folders/servers or removing connections is blocked while recovery is pending. Renewing credentials must not change the account that owns the files.

Before mounting, SDrive binds cache to the destination options and identifiers available in configuration, including the library/drive and remote folder. OAuth renewal that selects another library does not authorize sending old cache there. This binding does not prove account identity for providers that do not persist their own account identifiers: always renew with the same account.

The helper process belongs to a Windows Job Object, preventing it from running unsupervised after SDrive exits. After an unexpected exit, persistent files remain for recovery on the next launch.

## Distribution and reporting

Tool versions and files are pinned; tool files use SHA-256 and Go sources use module/SumDB checksums. Checksums do not prove that a binary exactly reproduces its source. Local builds do not have a publisher's digital signature.

The online installer includes SDrive and rclone and downloads WinFsp/WebView2 only when needed. The offline alternative includes all components. Downloads accept only HTTPS and files defined in the embedded manifest, with size and time limits. Partial files are never executed. Both variants verify SHA-256, Authenticode signature and publisher before executing each component; the file remains open without write/delete sharing until execution finishes. Signature trust uses local Windows certificates without mandatory online revocation checks.

Only the official WinFsp MSI requests elevation. SDrive and downloads run as the regular user. Uninstalling SDrive does not remove shared components. Installer-requested restarts are communicated, never forced.

The official rclone 1.75.2 binary uses Go 1.26.9 and was reported by `govulncheck` as affected by [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809), concerning compromised intermediate certificates. SDrive therefore rebuilds unmodified official sources with Go 1.26.10, which includes the fix.

There is a known discrepancy in OpenPGP analysis: `govulncheck -mode=binary` still reports [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932), while rclone source analysis with `-tags cmount` reports zero reachable vulnerabilities. The effective import tree does not include `golang.org/x/crypto/openpgp`; it uses the maintained `github.com/ProtonMail/go-crypto/openpgp` fork recommended by the advisory. The inference is a binary-analysis alert unconfirmed by source analysis; this is not proof of a general absence of vulnerabilities. Reassess when updating tools or dependencies.

To reproduce, use `govulncheck` v1.9.0 on SDrive and at the rclone 1.75.2 source root with `-tags cmount .`, as well as binary analysis. Initial SDrive and rebuilt rclone source analyses reported zero reachable vulnerabilities; binary analysis still reports the OpenPGP alert described above.

Never submit configuration dumps, tokens, keys, passwords, SAS URLs, cache directories or user files in issues. For suspected vulnerabilities, use a private repository advisory when available; in public issues, report only that a problem exists, without sensitive data or exploitation details.
