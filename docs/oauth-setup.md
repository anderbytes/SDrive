# User-owned OAuth applications

SDrive is a local tool. It supplies no shared OAuth application, client credentials, authentication relay or token service. For browser-based connections, register an application in your own provider account, then enter its Client ID and Client Secret in **Advanced → Access**. The supported registration flows below issue both values; they are required before SDrive starts authorization. Non-OAuth access remains in Basic, including GCS service accounts.

Application registration, approval and consent policies belong to you or your organization. This is SDrive's design choice, not a claim that every provider requires all applications to use personal registrations. A registration being configured does not certify that an account, tenant or region works.

Keep credentials out of screenshots, source control and support reports. Persistent connections store them in encrypted local configuration protected by Windows DPAPI. SDrive does not return saved Client Secrets to the form: leave the secret blank during renewal to keep it with the same Client ID; changing the ID requires the corresponding secret.

## Google Drive

1. Use a project you control in [Google Cloud Console](https://console.cloud.google.com/). Enable Google Drive API.
2. Configure Google Auth Platform consent branding, audience and data access. For an external application in Testing, add the intended Google account as a test user. Your organization may restrict consent.
3. Create an OAuth client of type **Desktop app**, then copy its Client ID and Client Secret into SDrive. The desktop loopback callback is `http://127.0.0.1:53682/`; do not substitute a web application's unrelated redirect configuration.
4. Writable drives request `https://www.googleapis.com/auth/drive`; read-only drives request `https://www.googleapis.com/auth/drive.readonly`. Register/approve only the access you intend to use. Restricted scopes may require provider verification for wider publication.
5. Authorize your original account and choose the intended shared drive, if applicable. An explicit root folder ID is available in Advanced options. Session-only access requires an explicit root/shared-drive identity; the default personal root does not prove account identity.

Renew using **Reconfigure account**. External Testing consent can expire; revocation or policy changes can also require renewed authorization. Preserve pending files and the original root/shared drive.

Reference: [rclone custom Google Drive clients](https://rclone.org/drive/#making-your-own-client-id), [Google desktop OAuth](https://developers.google.com/identity/protocols/oauth2/native-app).

## OneDrive and SharePoint Online

1. Register an application in your [Microsoft Entra tenant](https://entra.microsoft.com/). Choose the supported account types deliberately: your organization, multiple organizations, and/or personal Microsoft accounts as needed.
2. Add the **Web** redirect `http://localhost:53682/`. Create a client secret under Certificates & secrets and copy its **Value**, not its identifier. Store the expiration securely outside SDrive documentation.
3. Add delegated Microsoft Graph file permissions matching the pinned backend's consent: `Files.Read`, `Files.ReadWrite`, `Files.Read.All`, `Files.ReadWrite.All`, `offline_access` and `User.Read`. Site search additionally uses `Sites.Read.All`. Obtain administrator approval when your tenant requires it. A read-only mount does not reduce this backend's OAuth scopes.
4. Enter the Client ID and secret in SDrive, authorize the original account, and select the correct drive/library. SharePoint uses the same backend, with site/library selection.

Before a secret expires, renew it in your own registration and use Reconfigure account. An expired/deleted registration or denied tenant consent is an application-setup failure, not a reason to use a shared client. Recovery must retain the original drive/library ID.

Reference: [rclone Microsoft registration instructions](https://rclone.org/onedrive/#getting-your-own-client-id-and-key).

## Dropbox

1. Create a **Scoped access** application in the [Dropbox App Console](https://www.dropbox.com/developers/apps). Choose App folder or Full Dropbox according to the intended storage; this choice affects the visible root.
2. Add `http://localhost:53682/` to OAuth2 Redirect URIs. Copy App key as Client ID and App secret as Client Secret.
3. Enable the file scopes needed for listing/reading and, for writable access, writing: `files.metadata.read`, `files.content.read`, `files.metadata.write` and `files.content.write`; include `account_info.read` for account information. Reauthorize after changing granted scopes. A read-only mount does not itself rewrite the registration's scopes.
4. Authorize the intended account. Development-user limits and production approval remain your responsibility.

Renew through Reconfigure account after revocation, permission changes or credential rotation. Session-only access is unavailable here because this delivery does not add an independent Dropbox namespace-identity probe.

Reference: [rclone Dropbox registration instructions](https://rclone.org/dropbox/#get-your-own-dropbox-app-id).

## Box

1. In the [Box Developer Console](https://app.box.com/developers/console), create a custom application using **User Authentication (OAuth 2.0)**, not a server/JWT application.
2. Add `http://127.0.0.1:53682/` as an OAuth 2.0 redirect. Copy the Client ID and Client Secret.
3. Enable the application file/folder read scope and write scope only when needed. Submit the application for your enterprise's approval when required, then authorize the intended account in SDrive.

Use Reconfigure account after secret rotation or revoked consent. Session-only access remains unavailable without an independent root/account-identity probe.

Reference: [rclone Box registration instructions](https://rclone.org/box/#get-your-own-box-app-id).

## pCloud

1. Request/register an application through [pCloud's developer portal](https://docs.pcloud.com/). Provider approval may be required before credentials are issued.
2. Configure the redirect `http://localhost:53682/` and enter the issued Client ID and Client Secret in SDrive.
3. Select the region matching the account: `api.pcloud.com` for US or `eapi.pcloud.com` for Europe. The backend uses pCloud's OAuth file-access grant rather than a configurable scope list.
4. Authorize the intended account. Region selection is part of the destination and cannot be changed during pending recovery.

Renew through Reconfigure account after revocation or application credential changes. Session-only access remains unavailable without independent account/root verification.

Reference: [rclone pCloud](https://rclone.org/pcloud/), [pCloud OAuth](https://docs.pcloud.com/methods/oauth_2.0/).

## Google Cloud Storage

For **OAuth**, enable Cloud Storage API, configure Google consent/audience and create a **Desktop app** client in a project you control. Enter its Client ID and Client Secret under Advanced. The callback is `http://127.0.0.1:53682/`. The pinned backend requests `https://www.googleapis.com/auth/devstorage.read_write`; bucket IAM still determines effective access. Grant only the bucket permissions needed, and configure the relevant project number when using project-level bucket listing.

For **service-account access**, choose the separate Basic entry and select a JSON key file for a service account authorized for the intended bucket. This does not use OAuth client registration or browser sign-in. Keep that file protected and available at the selected path. Listing the project's buckets may require broader permissions than accessing a known bucket directly; enter a known bucket when listing is denied.

Renew OAuth consent or rotate the service-account key using Reconfigure account while preserving the original bucket/project. Session-only access is unavailable for GCS in this delivery.

Reference: [rclone Google Cloud Storage](https://rclone.org/googlecloudstorage/).

## Browser choice, cancellation and recovery

SDrive recognizes installed Edge, Chrome and Firefox, as well as the default browser. Private sign-in requires selecting a recognized browser explicitly. Private mode is never silently discarded; an unavailable browser produces an error. Browser launch is separate from account authorization, and an existing browser session can still affect account selection.

Use Cancel in SDrive to stop authorization; merely closing a browser tab does not signal completion to the local callback server. Cancellation before confirmation leaves the original connection intact. If pending data exist, sign in to the original destination and wait for uploads before disconnecting. Accounts, application approval, cancellation and renewal still require per-service live validation with explicitly selected test resources.

## Google Drive content selection

In the access step, **Google Drive contents** selects **My Drive / selected library** or **Shared with me**. The latter lists content other users shared with the signed-in account; it is separate from organizational Shared Drives and cannot be combined with a Shared Drive. Existing explicit root folders and library selections remain unchanged when the default view is retained. Select a folder after authorization or keep the selected view's root.

The choice remains in protected backend configuration and non-secret portable definitions. Imported definitions still require authorization. Recovery binds the view alongside the original root/path; changing it while recovery is pending is rejected. Read-only and provider permissions remain in force. Shared with me alone does not establish the verified destination needed for session-only access. Permission errors never select another view automatically.

Reference: [rclone Shared with me](https://rclone.org/drive/#drive-shared-with-me).

## Yandex Disk

1. Register an application you control in [Yandex OAuth](https://oauth.yandex.com/), using the API-access application type for your own Disk data. See [Yandex application registration](https://yandex.com/dev/id/doc/en/register-client).
2. Configure the callback `http://localhost:53682/` used by the pinned engine. Grant Disk API read access and the additional write permissions required for writable mounts; include storage-information access if you need quota reporting. Registration and available permissions depend on the provider.
3. Select **Advanced → Yandex Disk**, enter the application's Client ID and Client Secret, then authorize your intended account. Both fields are required for new connections; shared rclone credentials are never used.
4. Choose the destination folder and connect. Persistent secrets use the existing local encrypted storage. Use **Reconfigure account** to renew authorization; a blank secret is retained only for the same saved Client ID. Cancelled setup does not create a connection.

Session-only access, proxy routing and independent bandwidth controls are unavailable for this profile. Portable files contain definitions without OAuth credentials; imports require a new sign-in. Automated tests exercise setup and credential handling, but live Yandex registration, account access, renewal and mounted read/write remain unverified.

Reference: [rclone Yandex Disk](https://rclone.org/yandex/).
