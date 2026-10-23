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

The VFS cache is persistent. A potentially interrupted session is recorded before mounting. Pending files are checked through the rclone API and the local metadata `Dirty` field, including open files that have not entered the upload queue. `Dirty` must be an explicit JSON boolean: absent/null fields, invalid types, malformed metadata, links, unexpected cache roots and backing files without metadata never establish a clean cache. Critical `vfs/stats` fields must be present and correctly typed, with nonnegative counters and overflow-safe upload totals. These readers follow the pinned [rclone 1.75.2 cache statistics](https://github.com/rclone/rclone/blob/v1.75.2/vfs/vfscache/cache.go) and [metadata schema](https://github.com/rclone/rclone/blob/v1.75.2/vfs/vfscache/item.go); revalidate them on engine upgrades.

A snapshot carries `pending: null` when the complete pending count cannot be checked. The interface shows unknown status and a partial summary instead of interpreting a failed check as zero. Queued/active uploads and cache errors are separate: `erroredFiles` describes cache errors, not a count of failed uploads. Pending totals use the maximum of the durable dirty count, queued plus active uploads, and cache-error count; these sets may overlap. They are a conservative status indicator rather than an exact count of unique files across every source. There is no inferred completion percentage or per-connection transfer speed.

Display inspections, including failures, are reused for up to 30 seconds and show their sampling time. They are invalidated when connection lifecycle operations change the relevant state; overlapping inspections do not publish incomplete or invalidated results. Live engine statistics are still requested on every snapshot. Display samples never authorize disconnect or safe exit: disconnect checks fresh engine statistics and durable metadata, unmounts, then performs a new durable inspection before clearing the recovery flag. Safe exit also checks disconnected saved connections. Uncertainty keeps recovery protected and blocks a safe-exit declaration.

Inspection reads directories in batches of 128 entries, limits each metadata file to 8 MiB, and checks cancellation between entries and reads. Snapshot inspections share a two-second deadline; disconnect shares a 35-second deadline across its drain, unmount and final inspection. Incomplete/oversized inspections remain unknown; unusually large legitimate metadata may require a compatible reader or manual recovery. Local operating-system calls already in progress cannot be forcibly interrupted by these context checks.

Cache figures distinguish the configured target per connection, approximate cache use, free bytes available on the cache disk and the logical sizes of whole local dirty files. Mounted use comes from rclone `bytesUsed`; disconnected use is sampled with Windows allocated-size information so sparse files are not counted at their entire logical size. These are approximate measures, exclude filesystem overhead, and are not the bytes remaining to upload. Missing measurements appear as unavailable, independently of pending-state safety. Pending sizes require all dirty backing files to be measurable. Pending data remain preserved even above the configured target; there is no cache-delete action in this interface.

Before disconnecting, close files in other applications. There is no distributed transaction between an editor, Windows, cache and provider. Forced shutdown, disk failure or data an editor has not submitted to the system can cause losses SDrive cannot recover.

After an interruption, keep the cache and original destination. Changing folders/servers or removing connections is blocked while recovery is pending. Renewing credentials must not change the account that owns the files.

Before mounting, SDrive binds cache to the destination options and identifiers available in configuration, including the library/drive and remote folder. OAuth renewal that selects another library does not authorize sending old cache there. This binding does not prove account identity for providers that do not persist their own account identifiers: always renew with the same account.

The helper process belongs to a Windows Job Object, preventing it from running unsupervised after SDrive exits. After an unexpected exit, persistent files remain for recovery on the next launch.

## Local settings recovery

`settings.json` uses format version 1. The supported historical format has no version and may omit the language, which defaults to English. Both formats require a connections array and a settings object containing an explicit startup preference and cache limit. Empty objects, null values, incomplete structures and unsupported versions cannot authorize credential cleanup. Reading an old file does not migrate it; the next successful save writes version 1. An incompatible version requires a compatible application, rather than restoration by an older application.

Every settings save first preserves the previous valid file as `settings.backup.json`, then atomically replaces the primary file. The first save also creates the initial backup. A failed backup prevents replacement; an invalid existing primary is never overwritten by an ordinary save. Both files contain connection metadata, not encrypted credentials or cached file contents. They are not a backup of the whole profile. An intentional connection removal or credential replacement can make an older connection's credentials unavailable.

If local settings are damaged or an established profile's settings disappear, SDrive opens its recovery interface without starting the engine or creating keys. Restoration requires explicit confirmation and a compatible, validated local backup. The original file is retained as a unique `settings-damaged-*.json` archive. If preservation or replacement fails, restoration fails and the original data remain. Missing or unreadable backups require manual recovery; do not reset the profile or delete its files to dismiss the error.

Before restoration, SDrive writes `settings.recovery`. This durable protection suspends automatic mounting and orphan cleanup across restarts. Restored connections with existing cache require recovery at their saved destination; missing destination evidence blocks mounting. Recover and disconnect those drives, then choose **Verify recovery**. Verification requires all generated remotes and cache directories to belong to the restored connections, all required credentials to exist, and no active or pending recovery. Unknown credentials/cache remain preserved for manual recovery; they are never deleted to make verification pass. Editing/removing connections and creating replacement connections remain blocked during this protection, apart from renewing credentials for a connection already marked for recovery.

For manual recovery, exit without starting new writes and preserve the entire SDrive data directory, including the damaged archive, backup, `settings.recovery`, encrypted configuration, key and both cache trees. Reconstruct missing connection metadata only from trusted records of the original account and destination, using compatible software or maintainer assistance. Do not guess a destination, remove the recovery marker manually, or share the profile in an issue. Reopen SDrive and use **Verify recovery** once all data are accounted for. A lost DPAPI key cannot be reconstructed from the settings backup.

Restoration does not apply its Windows startup preference. Save preferences explicitly to reconcile it. A failed settings save restores the previous preference in memory and attempts startup rollback; failure of that rollback is reported together with the original error. Startup disagreement is also checked when reopening. Unit tests inject startup operations and never modify the real startup entry.

Keys and settings use synced temporary files followed by atomic promotion. Key creation never replaces an existing key; concurrent creation reuses the winning key. Missing keys beside existing encrypted configuration, unreadable configuration and empty/corrupt keys cause recovery errors rather than regeneration. Orphan-cleanup failures are separate maintenance warnings and can be retried without preventing use of saved connections. Cleanup preserves current/backup references and existing cache; unreadable evidence suspends deletion.

## Distribution and reporting

Tool versions and files are pinned; tool files use SHA-256 and Go sources use module/SumDB checksums. Checksums do not prove that a binary exactly reproduces its source. Local builds do not have a publisher's digital signature.

The online installer includes SDrive and rclone and downloads WinFsp/WebView2 only when needed. The offline alternative includes all components. Downloads accept only HTTPS and files defined in the embedded manifest, with size and time limits. Partial files are never executed. Both variants verify SHA-256, Authenticode signature and publisher before executing each component; the file remains open without write/delete sharing until execution finishes. Signature trust uses local Windows certificates without mandatory online revocation checks.

Only the official WinFsp MSI requests elevation. SDrive and downloads run as the regular user. Uninstalling SDrive does not remove shared components. Installer-requested restarts are communicated, never forced.

The official rclone 1.75.2 binary uses Go 1.26.9 and was reported by `govulncheck` as affected by [GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809), concerning compromised intermediate certificates. SDrive therefore rebuilds unmodified official sources with Go 1.26.10, which includes the fix.

There is a known discrepancy in OpenPGP analysis: `govulncheck -mode=binary` still reports [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932), while rclone source analysis with `-tags cmount` reports zero reachable vulnerabilities. The effective import tree does not include `golang.org/x/crypto/openpgp`; it uses the maintained `github.com/ProtonMail/go-crypto/openpgp` fork recommended by the advisory. The inference is a binary-analysis alert unconfirmed by source analysis; this is not proof of a general absence of vulnerabilities. Reassess when updating tools or dependencies.

To reproduce, use `govulncheck` v1.9.0 on SDrive and at the rclone 1.75.2 source root with `-tags cmount .`, as well as binary analysis. Initial SDrive and rebuilt rclone source analyses reported zero reachable vulnerabilities; binary analysis still reports the OpenPGP alert described above.

Never submit configuration dumps, tokens, keys, passwords, SAS URLs, cache directories or user files in issues. For suspected vulnerabilities, use a private repository advisory when available; in public issues, report only that a problem exists, without sensitive data or exploitation details.
