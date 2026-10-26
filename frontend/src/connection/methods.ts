import type { Connection, Provider } from "../api";

export const advancedDisclaimer = "These connections require additional setup. You will need to register an application with the provider and enter your Client ID and, when required, Client Secret. SDrive is a local tool and does not maintain shared OAuth applications. This model keeps application registration and permissions under your control. Read the service's setup instructions before continuing.";

export function methods(providers: Provider[]) {
  return providers.flatMap((p) => p.id === "gcs"
    ? [{ provider: p, authMethod: "serviceAccount", advanced: false, key: "gcs:serviceAccount", label: "Google Cloud Storage — service account" },
       { provider: p, authMethod: "oauth", advanced: true, key: "gcs:oauth", label: "Google Cloud Storage — OAuth" }]
    : [{ provider: p, authMethod: p.oauth ? "oauth" : "", advanced: p.oauth, key: p.id, label: p.name }]);
}

export function methodKey(c: Connection, providers: Provider[]) {
  void providers;
  return c.provider === "gcs" ? `gcs:${c.authMethod || "oauth"}` : c.provider;
}

export const oauthInstructions: Record<string, string> = {
  drive: "Register a Google desktop application, enable Drive API and configure the consent audience. Use drive or drive.readonly permission for the selected access mode.",
  onedrive: "Register a Microsoft application for your account type. Add the Web redirect http://localhost:53682/ and delegated Microsoft Graph file permissions and offline_access.",
  sharepoint: "Register a Microsoft application for your tenant. Add the Web redirect http://localhost:53682/, delegated file/site permissions and offline_access. Your administrator may need to approve consent.",
  dropbox: "Create a scoped Dropbox application with the intended folder access. Add http://localhost:53682/ as a redirect and enable the required file permissions before authorizing.",
  box: "Create a Box application using user OAuth 2.0. Add http://127.0.0.1:53682/ as a redirect and enable the required file permissions. Enterprise approval may be required.",
  pcloud: "Register a pCloud OAuth application and add http://localhost:53682/ as a redirect. Select the region matching your account. Application approval may be required.",
  yandex: "Register your own Yandex OAuth application for Disk API access. Add http://localhost:53682/ as the redirect URI and grant Disk read permissions, plus write permissions for writable access. Enter your Client ID and Client Secret before signing in.",
  gcs: "Enable Cloud Storage API, configure OAuth consent and create a Google desktop application. Grant only the bucket permissions needed by this account.",
};
