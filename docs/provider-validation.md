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
