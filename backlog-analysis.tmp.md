# Desktop feature backlog for analysis

This is a temporary assessment, not an implementation commitment or a replacement for `backlog.md`. Items 1–4 are included in the implementation scope; item 5 is excluded from this delivery. Remaining items are analysis candidates. Numbers identify analysis items only; source code and tests must use responsibility-based names.

## Scope and classification

SDrive is a local desktop application for an individual user. Retain remote-storage access as its existing core purpose, but exclude features requiring a new hosted SDrive service, central administration, cross-computer coordination or a corporate operating model. A local feature using the user's existing storage connection does not, by itself, introduce a hosted SDrive dependency.

Complexity estimates assume reuse of the current Wails application, isolated rclone workers, configuration wizard, protected storage and recovery rules. They are preliminary engineering assessments, not delivery estimates. Demand estimates are product judgments; no usage analytics or user research were collected.

- **Simple and direct:** existing engine/configuration capability, small interface and persistence changes, no major design decision expected.
- **Medium complexity:** bounded integration with a few explicit decisions and focused validation; no fundamental architecture change expected.
- **Complex / major decisions:** affects trust boundaries, native Explorer integration, recovery identity or file-operation semantics, or needs substantial engine work.
- **Somewhere in the future:** low expected demand, very specific environments, uncertain feasibility or disproportionate maintenance. This placement takes precedence over implementation simplicity.
- **Excluded:** outside the isolated desktop scope. These are not future commitments.

RaiDrive's release notes are historical product claims. They do not prove current availability, compatibility or performance. Existing SDrive behavior was inspected in the preceding comparison; no new provider or native-runtime certification is implied here.

## Simple and direct

### 1. Select the download format for native Google documents

- **Previous comparison:** item 10; RaiDrive 2025 notes.
- **Current behavior:** the engine already exports to DOCX, XLSX, PPTX and SVG by default; there is no PDF preference in SDrive.
- **Smallest scope:** expose the engine's preferred export formats for a Google Drive connection, including PDF. Do not build a document converter or upload/import conversion feature.
- **Why simple:** reuse an existing backend option and the established connection form and persistence flow.
- **Boundary to check:** format selection changes visible filenames/extensions. Apply after a clean disconnect; block changes during pending recovery and ensure the destination/cache binding cannot reuse incompatible pending data. If that requires broader identity migration, reclassify before implementing.
- **Acceptance:** default behavior stays unchanged; selection survives reopening and portable definitions; document listings and downloaded content match the selected format. Record live-provider acceptance separately.

### 2. Extend the maximum cache retention to 180 days

- **Previous comparison:** part of item 24; RaiDrive 2024 notes.
- **Current behavior:** retention is restricted to 1–720 hours, or 30 days.
- **Smallest scope:** increase the supported finite maximum to 4,320 hours and update validation, localized help and affected checks.
- **Why simple:** the duration option, persistence and engine mapping already exist.
- **Acceptance:** old preferences keep their behavior; the new maximum survives Save/reopen; values outside the range fail validation; pending uploads remain protected. Retention is an eviction policy, not a promise to keep files available offline.
- **Excluded from this item:** disabling expiration and zero-age semantics; see item 18.

### 3. Configure the virtual capacity reported to Explorer

- **Previous comparison:** item 25; RaiDrive 2020 notes.
- **Current behavior:** SDrive displays provider-reported capacity but offers no user-selected virtual total.
- **Smallest scope:** expose the existing VFS virtual disk-space option, subject to confirming its exact behavior in the pinned engine.
- **Why simple:** expected to be a numeric connection option, with no new quota service or remote scan.
- **Acceptance:** distinguish the presentation value from the real provider quota and cache free space; unset preserves defaults; reject invalid/overflowing values; verify native Explorer reporting after reconnect. Never describe this as reserved space or an enforced quota.

## Medium complexity

### 4. Add SOCKS5 proxy support

- **Previous comparison:** item 12; RaiDrive 2025 notes.
- **Scope:** extend the current encrypted proxy settings and supported HTTP-backend routing, including authentication and an explicit hostname-resolution policy.
- **Why medium:** reuses proxy storage and worker configuration, but the effective transport behavior needs verification before expanding accepted URL schemes.
- **Decisions:** local versus proxy-side DNS; supported services; whether OAuth authorization/token requests follow the same route.
- **Acceptance:** disposable proxy tests cover DNS, credentials, failure and reconnect; local RPC stays direct; diagnostics redact credentials; unsupported protocols remain unavailable rather than silently bypassing the proxy.

### 5. Import a static Windows proxy configuration

- **Decision:** excluded from this delivery; no Windows proxy import is implemented.

- **Previous comparison:** item 13; RaiDrive 2019 notes.
- **Scope:** an explicit action reads the current user's supported Windows proxy settings into the existing form. The user reviews and saves them.
- **Why medium:** local platform integration with bounded parsing and mapping; no new routing subsystem is necessary for a static proxy.
- **Decisions:** which Windows settings source to use and how to present protocol-specific proxy lists and bypass rules.
- **Acceptance:** disabled/missing/unsupported settings are explained; existing credentials are not overwritten or inferred; Cancel changes nothing. Never silently ignore bypass semantics or copy a PAC URL as a proxy address.
- **Excluded from this item:** automatic PAC/WPAD execution; see item 17.

## Complex or requiring major decisions

### 6. Add Google file actions to the Explorer context menu

- **Previous comparison:** item 9; RaiDrive 2025 notes.
- **Scope:** copy the corresponding web link and open it in the browser for a selected Google Drive file.
- **Why complex:** the original requirement concerns Explorer, requiring Windows Shell integration, selected-path validation and a reliable local-path-to-remote-ID mapping. Adding a button inside SDrive is a smaller feature, but does not fulfill the same requirement.
- **Decisions:** native integration mechanism, supported Windows versions, registration ownership/cleanup, multi-selection and representation of native Google documents.
- **Acceptance:** validate that selections belong to the intended active mount; reject stale/ambiguous paths; avoid exposing credentials or accepting arbitrary bridge commands; uninstall removes only owned integration. Use the existing provider connection, with no SDrive-hosted link service.

### 7. Support genuine SFTP multifactor authentication

- **Previous comparison:** item 19; RaiDrive 2018/2022 notes.
- **Scope:** handle distinct keyboard-interactive challenges such as password plus OTP and supported external approval workflows.
- **Why complex:** the pinned engine currently answers keyboard-interactive questions with the same password; adding a password field or reusing the generic configuration wizard does not implement a live SSH challenge exchange.
- **Decisions:** engine support versus a maintained patch; challenge lifetime, reconnect behavior, cancellation and approval timeouts.
- **Acceptance:** a disposable MFA SSH fixture exercises distinct answers, cancellation, expired codes and reconnect without persisting OTPs or exposing prompts/secrets in diagnostics. Do not promise every Duo policy.

### 8. Export and import credentials securely between computers

- **Previous comparison:** credential portability portion of item 23; RaiDrive 2024 notes.
- **Scope:** an optional local, encrypted backup opened by the individual user on another machine. No online vault or synchronization service.
- **Why complex:** the existing portable format deliberately excludes credentials and Windows DPAPI cannot simply be copied to another user/computer.
- **Decisions:** whether credential portability belongs in the product; backup format, password-based encryption, key derivation, recovery expectations, overwrite/conflict behavior and treatment of OAuth/key-file dependencies.
- **Acceptance:** unauthenticated or damaged backups fail closed; secrets stay out of plaintext exports/logs; existing configurations remain intact on failure; imports never transfer pending uploads or authorize recovery to another destination.
- **Recommendation:** decide the product/security boundary before implementation. Keep the existing credential-free export available.

### 9. Copy files within a mount without downloading and uploading their content

- **Previous comparison:** missing copy portion of item 6; RaiDrive 2019–2026 notes.
- **Current behavior:** remote rename/move/delete already work where supported; the observed Windows Shell copy over WebDAV reads and uploads content.
- **Why complex:** a backend offering server-side COPY does not make ordinary Explorer read/write calls become a remote copy. The hard part is integrating copy/offload semantics into the mounted filesystem or another explicit user flow.
- **Decisions:** Explorer compatibility versus a separate explicit application action; engine/WinFsp support; safe fallback, overwrite, cancellation and cross-backend behavior.
- **Acceptance:** instrumented disposable storage proves no content transfer in supported cases; collision, partial failure and cancellation preserve data; unsupported cases use an explicit safe fallback. No distributed coordination service is needed.

### 10. Add alternate SFTP and FTP endpoints safely

- **Previous comparison:** missing protocol coverage from item 14; RaiDrive 2021 notes.
- **Current behavior:** alternate HTTPS WebDAV addresses require the same verified remote resource identity.
- **Why complex:** matching a host key, certificate, username or folder name does not establish that two endpoints refer to the same remote filesystem and recovery destination.
- **Decisions:** whether each protocol can establish sufficient identity; when switching is permitted; handling pending writes and unequal endpoint capabilities.
- **Acceptance:** reject mismatched or unverifiable destinations; never replay pending data to an unproven alternative; retain primary-endpoint recovery. Do not silently weaken the WebDAV identity policy or switch a live mount underneath open files.

## Somewhere in the future

These items are intentionally outside the present implementation shortlist. Reconsider them only with concrete demand or stronger feasibility evidence.

| No. | Candidate | Previous comparison | Estimated complexity | Reason to defer / revisit trigger |
| --- | --- | --- | --- | --- |
| 11 | TPM-backed connection protection | 4 | High | DPAPI protection already exists. Consider a separate hardware-bound key design only after defining a threat model and recovery behavior that justify the extra restriction. |
| 12 | Mount before Windows login | 2 | High | A service introduces another identity, credential access, lifecycle and UI/process boundary. Revisit only for a demonstrated personal unattended-use requirement. |
| 13 | Windows Arm64 delivery | 5 | Medium–high | Needs compatible application, engine, dependencies, installers and real hardware validation. Revisit when there are actual Arm users and a maintainable validation environment. |
| 14 | x86, older Windows and Windows Server support | 5 | High | Expands platform/dependency and installer maintenance for a narrow audience. Requires a deliberate support-policy change, not just another build target. |
| 15 | Continuous per-process file I/O monitor | 7 | High | Considerably more intrusive and expensive than the existing selected-file Restart Manager query. Revisit only if that query demonstrably fails a recurring user need. |
| 16 | Per-drive preview/thumbnail suppression | 8 | High / uncertain | First determine whether Explorer/WinFsp provides a reliable local mechanism without blocking ordinary reads or changing global Explorer preferences. There is no confirmed implementation path yet. |
| 17 | Automatic proxy discovery, PAC and WPAD | Extension of 13 | High | A dynamic routing policy is much broader than importing a static address. Revisit only when static proxy setup cannot serve actual users. |
| 18 | Indefinite or zero-age cache retention | Remaining part of 24 | Medium | Semantics must be defined separately: zero engine age is not automatically immediate eviction, and neither setting means full offline synchronization. Revisit with a concrete storage-management need. |
| 19 | FTP active mode | 16 | High | Not supported by the pinned engine; uncommon legacy-server demand and firewall complexity. Revisit for a selected server that cannot use passive mode. |
| 20 | FTP Mode Z compression | 17 | High | Missing in the pinned engine and useful only with compatible servers/workloads. Needs evidence of benefit before maintaining protocol changes. |
| 21 | Legacy SFTP/FTP charset conversion | 18 | High | Must preserve names reversibly and prevent collisions or destructive renames. Revisit with a reproducible server/encoding fixture; escaping is not transcoding. |
| 22 | WebDAV NTLM for IIS | 20 | Medium | Mainly a specialized server deployment. If real individual-user demand exists, investigate the engine's existing NTLM support with a disposable IIS fixture. |
| 23 | User-selectable TLS versions | 21 | Medium | No present need beyond secure engine negotiation and existing FTPS mode selection. Revisit for a reproducible compatibility case without silently enabling obsolete protocols. |
| 24 | OneDrive/SharePoint China | 22 | Medium | Region-specific OAuth registration, endpoints and account validation. Revisit with a user-owned application and a selected account in that region. |
| 25 | Synology local-IP discovery | 15 | Medium–high | Requires a supported discovery/identity mechanism and hardware acceptance. Manual addressing already works; do not infer identity from a matching NAS name. |
| 26 | OneNote integration | 11 | High / uncertain | The pinned engine does not expose notebooks as normal readable files. Define the intended behavior before promising notebook editing or synchronization. |
| 27 | Interface font selection | Part of 26 | Low–medium | A local preference is feasible but adds layout/accessibility validation with limited demonstrated demand. Revisit on an actual readability requirement. |
| 28 | Additional interface languages | Remaining part of 26 | Medium, ongoing | Seven languages already exist. Add individual languages in response to actual users and maintain translations, native menus and installers together. |
| 29 | Send a configuration export through the local email application | Email portion of 23 | Low–medium | A convenience action with limited value over attaching the exported file manually. If revisited, use a user-controlled local compose flow; no sending service or automatic delivery. |
| 30 | Additional dedicated provider integrations | 28 | Per provider: medium–high | No bulk provider expansion. Revisit SAS LSAF, Filen, SugarSync, Koofr, OpenDrive, Ulož.to Disk, Synology C2, DirectCloud, Oracle OCI, Mail.ru Cloud, NAVER MYBOX and Google Photos individually, with actual demand, available APIs and user-owned authentication. First check existing S3/WebDAV/SMB coverage; historical RaiDrive support does not establish current API feasibility. |
| 31 | Dedicated Azure Files flow or branded S3 presets | Qualification after 28 | Low–medium to high | These were not confirmed functional gaps: generic SMB/S3 may already provide access. Require evidence of a missing capability before adding another profile. |
| 32 | Explicit WSL/Dev Drive compatibility | Previously unconfirmed | Medium–high | Measure the actual compatibility gap with disposable mounts before considering changes. Current engine behavior may already satisfy some uses. |
| 33 | Citrix compatibility | Previously unconfirmed | High / uncertain | Very specialized and often corporate. No corporate deployment work; revisit only for an explicitly scoped individual desktop scenario. |
| 34 | Preserve thumbnails across reconnects | Previously unconfirmed | Medium / uncertain | Windows cache behavior may already cover part of this. Establish a reproducible missing behavior before introducing another cache or shell integration. |
| 35 | Additional OneDrive shared-link/shortcut and Google Computers views | Previously unconfirmed | Medium–high | Ordinary shared content and configured roots must be distinguished from special provider views. Verify each scenario first; do not count inherited engine support as missing functionality. |
| 36 | Faster external-change visibility and instant file properties | Previously unconfirmed | Medium–high | Existing metadata expiry, backend polling and explicit refresh already handle remote changes. Revisit with measured stale behavior and request-cost limits; no push service, real-time guarantee or central coordinator. |

## Excluded from the isolated desktop backlog

These entries document the filtering decision; they are not actionable backlog items.

| No. | Removed scope | Previous comparison | Reason |
| --- | --- | --- | --- |
| 37 | Cross-computer editing locks and coordinated shared editing | 1 | Requires coordination across independent clients or provider-specific remote locking. Explicitly outside the requested isolated operating model. Existing local file-sharing restrictions remain relevant. |
| 38 | Public mounts for other Windows users or shared organizational use | 3 | Cross-user access and permission management do not fit the individual-user scope. A private pre-login service, if ever justified, is a separate future item. |
| 39 | Central administration, managed policy and corporate defaults | Managed portion of 27 | Requires an organizational control plane and corporate product scope. |
| 40 | Online update notification/catalog service | Update portion of 27 | Depends on an online release source. This request excludes new online functionality; retain manual installer-based updates. |
| 41 | Cloud credential backup/sync, hosted link generation or hosted email delivery | Possible expansions of 8, 23 and 29 | Would introduce external application services. Local encrypted backup, existing-provider links and user-controlled local email composition are the only retained interpretations. |
| 42 | Licenses, subscriptions, mandatory product accounts and team management | Other commercial announcements in the release history | Commercial/corporate mechanisms are outside this desktop feature backlog and were not functional requirements for SDrive. |

## Analysis and implementation rules

1. Analyze one numbered candidate at a time. Confirm demand, the exact user-visible behavior and current engine capability before accepting implementation.
2. Use the smallest complete change. Do not introduce a hosted component, provider relay, shared OAuth client or corporate feature to close a historical parity gap.
3. Preserve pending uploads, original recovery destinations, protected credentials and unrelated working-tree changes. Unknown state never means clean.
4. Carry accepted connection options through setup, validation, persistence, reauthentication, portable definitions and recovery binding where applicable. A new field is not complete when only its control appears in the UI.
5. Keep source and documentation in English; add user-facing text in English and Brazilian Portuguese and preserve keyboard accessibility.
6. Run focused checks appropriate to accepted work. Distinguish simulated tests from native mount checks and live-provider acceptance; use explicitly selected disposable resources for the latter.
7. Every application implementation must run `./scripts/build.ps1 -Installer`, regenerate the source archive and both installers, and verify every recorded artifact hash. This document alone does not require a build.

## Evidence and traceability

The previous comparison used the annual RaiDrive release-note URLs for [2026](https://www.raidrive.com/pt-BR/release-notes/2026), [2025](https://www.raidrive.com/pt-BR/release-notes/2025), [2024](https://www.raidrive.com/pt-BR/release-notes/2024), [2023](https://www.raidrive.com/pt-BR/release-notes/2023), [2022](https://www.raidrive.com/pt-BR/release-notes/2022), [2021](https://www.raidrive.com/pt-BR/release-notes/2021), [2020](https://www.raidrive.com/pt-BR/release-notes/2020), [2019](https://www.raidrive.com/pt-BR/release-notes/2019), [2018](https://www.raidrive.com/pt-BR/release-notes/2018) and [2017](https://www.raidrive.com/pt-BR/release-notes/2017). These returned a consolidated history during inspection. This file reorganizes that comparison; it is not a second live audit of those pages.

Local reference points: [catalog](internal/core/catalog.go), [mount options](internal/core/mount_target.go), [network validation](internal/core/network.go), [portable configuration](internal/core/portability.go), [connectivity limits](docs/connectivity.md), [mount behavior](docs/mount-runtime.md), [file operations and shared-editing limits](docs/access-and-file-operations.md), [security boundaries](docs/security.md) and [desktop preferences](docs/portability-and-desktop.md).

Every numbered entry from the previous comparison is accounted for in the `Previous comparison` fields. Previously bundled entries were split when their complexity or scope differed. Items that were explicitly unconfirmed remain investigation-only, never confirmed missing features.

## Implementation evidence for items 1–3

- Implemented Google document format selection through provider configuration, field reload, portable definitions and recovery identity. Default recovery hashes remain compatible. `TestDocumentFormatsSetupFieldsAndPortableDefinitions`, `TestDocumentFormatsBindRecoveryAndPreserveDefaultIdentity`, `TestDocumentFormatRecoveryAllowsOnlyOriginalFormat` and `TestDocumentFormatChangeRequiresCleanDisconnectedCache` cover simulated setup and recovery. Live Google listing/content acceptance remains unverified because no disposable Google resource was selected.
- Extended finite cache retention to 4,320 hours with unchanged defaults and pending-upload protection. Validation boundaries, saved/reopened settings, engine duration mapping and the Preferences save flow are covered.
- Implemented virtual Explorer capacity with integer/overflow validation and persistence in local and portable configuration. `TestWindowsVirtualCapacityAfterReconnect` passed on a disposable native WinFsp/WebDAV mount: Windows reported 256 GiB, then 512 GiB after reconnect, and the engine default after resetting to zero. This uses the Windows volume API consumed by Explorer; the Explorer properties dialog was not inspected manually.
- User-facing additions are translated in all seven supported languages. See `docs/mount-runtime.md` for semantics and limits.

## Implementation evidence for SOCKS5 proxy support

- Added `socks5://` and `socks5h://` to the existing encrypted proxy preference. Both use proxy-side destination DNS; authentication is optional with bounded username/password fields. Existing supported HTTP services and direct local RPC remain unchanged. Browser authorization uses the browser network settings.
- `TestSOCKSProxyValidationAndCredentialPersistence` and `TestSOCKSProxyDestinationVerification` cover validation, protected credential views/preservation, HTTPS destination identity and failure without direct fallback.
- `TestSOCKSProxyTransfersDNSFailuresAndReconnect` passed against the actual pinned engine using disposable loopback SOCKS5/WebDAV fixtures. Both schemes passed with and without authentication, including upload/download content checks, DNS through the proxy, process reconnection, incorrect passwords, unavailable proxy, direct-RPC availability and credential-free diagnostics. No external proxy is required.
- Preferences tests cover saved SOCKS settings and editing in English and Brazilian Portuguese; additions are translated in all seven application languages. External proxies and live-provider OAuth remain unverified.
- Item 5 is excluded from this delivery.
