import type { Wizard } from "./useConnectionWizard";

export function DirectoryCacheOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, initial } = wizard;
  return <>
    <label>{t("Virtual capacity in Explorer (GiB)")}<input type="number" min="0" max="8388607" step="1" disabled={initial.recovery} {...validationProps("virtualCapacityGiB")} value={c.virtualCapacityGiB || 0} onChange={(e) => change({ virtualCapacityGiB: Number(e.target.value) })} /></label>
    {fieldFeedback("virtualCapacityGiB")}
    <small>{t("Zero uses the provider default. This changes the displayed total after reconnecting; it does not reserve space, enforce a quota or change provider quota or local cache free space.")}</small>
    <label>{t("Directory metadata lifetime (seconds)")}<input type="number" min="1" max="86400" step="1" disabled={initial.recovery} {...validationProps("directoryCacheSeconds")} value={c.directoryCacheSeconds || 300} onChange={(e) => change({ directoryCacheSeconds: Number(e.target.value) })} /></label>
    {fieldFeedback("directoryCacheSeconds")}
    <small>{t("External changes appear on the next directory access after this lifetime, or after Refresh files in Explorer. Some services detect changes earlier. This is separate from SDrive status updates and applies on the next connection.")}</small>
  </>;
}

export function NetworkOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, initial } = wizard;
  const dav = ["webdav", "nextcloud", "owncloud"].includes(c.provider);
  const proxySupported = dav || ["drive", "onedrive", "sharepoint", "dropbox", "box", "pcloud", "s3", "b2", "azureblob", "gcs"].includes(c.provider);
  return <>
    <p>{t("Network changes apply on the next connection. Zero means unlimited or the engine default.")}</p>
    {(["downloadLimit", "uploadLimit"] as const).map((key) => <label key={key}>
      {t(key === "downloadLimit" ? "Download limit (KiB/s)" : "Upload limit (KiB/s)")}
      <input type="number" min="0" max="1073741824" step="1" disabled={!proxySupported || initial.recovery} {...validationProps("bandwidth")} value={(c[key] || 0) / 1024} onChange={(e) => change({ [key]: Number(e.target.value) * 1024 })} />
    </label>)}
    {fieldFeedback("bandwidth")}
    {!proxySupported && <small>{t("Independent bandwidth limits are supported for HTTP storage services only.")}</small>}
    <label className="check"><input type="checkbox" {...validationProps("useProxy")} checked={c.useProxy || false} disabled={!proxySupported || initial.recovery} onChange={(e) => change({ useProxy: e.target.checked })} />{t("Use the proxy configured in Preferences")}</label>
    {!proxySupported && <small>{t("Proxy routing is not supported for this service.")}</small>}
    {fieldFeedback("useProxy")}
    {dav && <>
      {(["connectTimeout", "idleTimeout"] as const).map((key) => <label key={key}>{t(key === "connectTimeout" ? "Connection timeout (seconds)" : "HTTP idle timeout (seconds)")}<input type="number" min="0" max="3600" disabled={initial.recovery} {...validationProps(key)} value={c[key] || 0} onChange={(e) => change({ [key]: Number(e.target.value) })} />{fieldFeedback(key)}</label>)}
      <label>{t("Alternate HTTPS WebDAV address")}<input type="url" {...validationProps("alternateURL")} value={c.alternateURL || ""} disabled={initial.recovery || !wizard.auth} onChange={(e) => change({ alternateURL: e.target.value })} /></label>
      {fieldFeedback("alternateURL")}
      <small>{t("Both addresses must expose the same DAV:resource-id for the selected folder. Fallback is checked only when connecting and never replaces an active mount.")}</small>
    </>}
  </>;
}

export function ConnectionOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, browsers, initial, forgetCredentials, setForgetCredentials } = wizard;
  const sessionSupported = ["webdav", "nextcloud", "owncloud", "drive", "onedrive", "sharepoint"].includes(c.provider);
  return <>
    <DirectoryCacheOptions wizard={wizard} />
    <NetworkOptions wizard={wizard} />
    {wizard.auth && <>
      {wizard.p.oauth && c.authMethod !== "serviceAccount" && <>
        <label>{t("Sign-in browser")}<select {...validationProps("browser")} value={c.browser || ""} onChange={(e) => change({ browser: e.target.value })}>{browsers.map((b) => <option key={b.id} value={b.id}>{t(b.name)}</option>)}{c.browser && !browsers.some((b) => b.id === c.browser) && <option value={c.browser}>{t("Selected browser unavailable")}</option>}</select></label>
        {fieldFeedback("browser")}
        <label className="check"><input type="checkbox" checked={c.privateBrowser || false} onChange={(e) => change({ privateBrowser: e.target.checked })} />{t("Open a private sign-in window")}</label>
      </>}
      <label className="check"><input type="checkbox" checked={c.sessionOnly || false} disabled={!sessionSupported || initial.recovery} onChange={(e) => change({ sessionOnly: e.target.checked, autoMount: e.target.checked ? false : c.autoMount })} />{t("Sign in on every connection (credentials only in memory)")}</label>
      {fieldFeedback("sessionOnly")}
      <small>{t("Session-only sign-in requires a verifiable destination. After disconnecting or reopening, sign in again to the original storage. Browser sessions may still be reused.")}</small>
      {c.sessionOnly && initial.id && !initial.sessionOnly && <label className="check"><input type="checkbox" required checked={forgetCredentials} onChange={(e) => setForgetCredentials(e.target.checked)} />{t("Remove this connection's saved credentials when setup is confirmed")}</label>}
    </>}
  </>;
}
