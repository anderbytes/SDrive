# Provider validation matrix

**Implemented setup** and **mocked tests** do not mean validation with a real account. Do not report third-party results without running the scenarios and recording sanitized evidence.

This development distribution has empty project OAuth registrations. Google Drive, OneDrive, SharePoint, Dropbox, Box, pCloud and Google Cloud Storage OAuth sign-in are unavailable without configured application registrations. GCS also supports service account files. Maintainers configure identities as described in the [development guide](development.md); regular users do not need to create a Client ID.

| Profile | rclone backend | Setup and automated tests | Live service |
| --- | --- | --- | --- |
| Google Drive | drive | Yes | Not verified |
| OneDrive | onedrive | Yes | Not verified |
| SharePoint Online | onedrive | Yes | Not verified |
| Dropbox | dropbox | Yes | Not verified |
| Box | box | Yes | Not verified |
| pCloud | pcloud | Yes | Not verified |
| MEGA | mega | Yes | Not verified |
| Proton Drive | protondrive, beta | Yes | Not verified |
| Nextcloud | webdav, nextcloud | Yes | Not verified |
| ownCloud | webdav, owncloud | Yes | Not verified |
| WebDAV | webdav, other | Yes | Not verified |
| SFTP | sftp | Yes | Not verified |
| FTP / FTPS | ftp | Yes | Not verified |
| SMB / CIFS | smb | Yes | Not verified |
| S3 / compatible services | s3 | Yes | Not verified |
| Backblaze B2 | b2 | Yes | Not verified |
| Azure Blob | azureblob | Yes | Not verified |
| Google Cloud Storage | google cloud storage | Yes | Not verified |

Tests also compare field names for all 18 profiles against the real rclone version's catalog. Real mounting requires WinFsp and an interactive Windows session; tests without that prerequisite are reported as skipped.

The [integration suite](development.md#recovery-and-filesystem-integration-checks) uses a disposable loopback WebDAV server, synthetic files and isolated encrypted profiles. Engine-only service/configuration and content checks do not certify a live provider. Native tests cover service mounting, interrupted pending uploads at the original destination, open handles/late writes and filesystem content. Native checks require compatible WinFsp and explicit opt-in. Synthetic fixtures do not change the live-service validation status above.

New OAuth connections without project registration are stopped at service selection. GCS remains selectable because a service account JSON file does not require project OAuth. Existing connections with a custom OAuth identity can still renew. The form explains SSH private keys/passphrases, Azure account keys versus SAS URLs, and GCS service accounts; this guidance does not certify those authentication modes against a live account.

## Recovery identity limits

The pinned rclone 1.75.2 source has no `UserInfo` implementation for the 18 SDrive profiles. No generic account-identity probe is added. Recovery checks the existing destination binding both after renewal and before mounting. This protects the identifiers below when present; it does not prove who owns an account or that a credential still grants access. Always renew with the original account and destination.

| Profiles | Evidence used in the existing destination hash | Remaining limit |
| --- | --- | --- |
| Google Drive | Configured root folder/shared drive and selected path | Default personal root does not identify the signed-in account |
| OneDrive, SharePoint Online | Drive/library ID and type, configured site/root and path | Library binding is not proof of account ownership |
| Dropbox | Profile and path, plus any already configured destination fields | Default guided configuration does not persist an account ID; namespace identity is not independently verified |
| Box | Configured root folder and path | Default root is shared across account configurations and does not identify the account |
| pCloud | Region hostname, configured root and path when present | Region/default root do not identify the signed-in account |
| MEGA, Proton Drive | Configured username and path | Username is configuration evidence, not independent account verification; session tokens are excluded |
| Nextcloud, ownCloud, WebDAV | URL, vendor, username and path | The server controls what those credentials address |
| SFTP, FTP / FTPS, SMB / CIFS | Host, configured port, username and path | Server/user configuration is not proof of ownership; FTP transport is separately preserved during renewal |
| S3 / compatible services | Provider, endpoint, region and path | Rotated access keys can address a different account; account identity is not available |
| Backblaze B2 | Backend and selected path | Key ID is deliberately excluded to allow rotation; no stable account ID is persisted in guided configuration |
| Azure Blob | Configured account, endpoint and path | SAS host/container are also constrained during recovery renewal; this does not prove the signing identity |
| Google Cloud Storage | Configured project identifiers and path | OAuth/service-account credential changes do not prove identity; credential file paths are excluded |

The check is based on [rclone 1.75.2 backend implementations](https://github.com/rclone/rclone/tree/v1.75.2/backend) and the existing SDrive hash fields, not live provider certification. Hash format and settings format are unchanged. Stronger proof for profiles without stable account identifiers requires separately scoped provider integration and real-account validation.

Local regressions cover all FTP modes, renewal consent, invalid configuration responses, saved-remote checks, recovery library mismatch, cancellation during verification, sensitive questions and UI behavior. A temporary real-rclone test checks the completion response and FTP configuration persistence without contacting a provider. Recognized error signals are categorized conservatively; unrecognized provider responses retain generic feedback. All live-service entries above remain **Not verified**.

## Checklist for each contribution

Use the **Provider validation** issue and a disposable test folder/bucket. **Test access** only reads data; contributors must deliberately perform the write scenarios below.

- [ ] New setup and cancellation without replacing another account.
- [ ] Authentication, MFA where applicable and browser cancellation.
- [ ] Application reopening and token/credential renewal.
- [ ] Correct region, personal/business account, shared drive or library selection.
- [ ] Folder browsing after authentication; direct destination works when the account cannot list the root.
- [ ] Destination validated before saving and mounting; cancelling after authentication preserves the previous connection.
- [ ] Listing and mounting at the selected letter; an occupied letter produces an error.
- [ ] Opening an existing file, reading a large file and using Unicode names/spaces.
- [ ] Creating, editing, renaming and deleting disposable files only.
- [ ] Reading/writing with your everyday application; checking provider content after upload.
- [ ] Read-only mode blocks writes.
- [ ] Network loss and recovery during uploads preserves pending files.
- [ ] Closing files, waiting for uploads and unmounting; no orphaned helper process.
- [ ] SDrive interruption: recover the original connection without changing the destination or deleting cache.
- [ ] Tray, optional Windows startup and automatic mounting per connection.
- [ ] English default, language switching and saved language after reopening.
- [ ] Logs, screenshots and preferences do not expose secrets.

Record **passed / failed / not applicable / not run** for each scenario. A personal account does not automatically validate business accounts, other tenants, other regions or every S3/WebDAV-compatible implementation.

Include SDrive, rclone, WinFsp and Windows versions; account/server type; scenario; expected/observed result; evidence without private names, token-bearing URLs or credentials.
