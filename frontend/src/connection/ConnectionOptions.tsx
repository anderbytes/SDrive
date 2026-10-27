import { FieldHelp } from "../FieldHelp";
import type { Wizard } from "./useConnectionWizard";
import { supportsNetworkOptions } from "./methods";

export function DirectoryCacheOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, initial } = wizard;
  return <>
    <div className="option-with-help"><label className="check"><input type="checkbox" checked={c.showHiddenFiles !== false} disabled={initial.recovery || wizard.data.platform === "linux"} onChange={(e) => change({ showHiddenFiles: e.target.checked })} />{t("Show hidden files")}</label>
      <FieldHelp label={t("Show hidden files")}>{t("When unchecked, names starting with a dot are marked hidden. Explorer decides whether to display them. Applies on the next connection.")}</FieldHelp>
    </div>
    <div className="field-with-help">
      <label><span className="field-label">{t("Virtual capacity in Explorer (GiB)")}</span><input type="number" min="0" max="8388607" step="1" disabled={initial.recovery} {...validationProps("virtualCapacityGiB")} value={c.virtualCapacityGiB || 0} onChange={(e) => change({ virtualCapacityGiB: Number(e.target.value) })} /></label>
      {fieldFeedback("virtualCapacityGiB")}
      <FieldHelp label={t("Virtual capacity in Explorer (GiB)")}>{t("Zero uses the provider default. This changes the displayed total after reconnecting; it does not reserve space, enforce a quota or change provider quota or local cache free space.")}</FieldHelp>
    </div>
    <div className="field-with-help">
      <label><span className="field-label">{t("Directory metadata lifetime (seconds)")}</span><input type="number" min="1" max="86400" step="1" disabled={initial.recovery} {...validationProps("directoryCacheSeconds")} value={c.directoryCacheSeconds || 300} onChange={(e) => change({ directoryCacheSeconds: Number(e.target.value) })} /></label>
      {fieldFeedback("directoryCacheSeconds")}
      <FieldHelp label={t("Directory metadata lifetime (seconds)")}>{t("External changes appear on the next directory access after this lifetime, or after Refresh files in Explorer. Some services detect changes earlier. This is separate from SDrive status updates and applies on the next connection.")}</FieldHelp>
    </div>
  </>;
}

export function NetworkOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, initial } = wizard;
  const dav = ["webdav", "nextcloud", "owncloud"].includes(c.provider);
  if (!supportsNetworkOptions(c.provider)) return null;
  return <>
    <div className="option-with-help"><span>{t("Network")}</span><FieldHelp label={t("Network")}>{t("Network changes apply on the next connection. Zero means unlimited or the engine default.")}</FieldHelp></div>
    {(["downloadLimit", "uploadLimit"] as const).map((key) => <label key={key}>
      {t(key === "downloadLimit" ? "Download limit (KiB/s)" : "Upload limit (KiB/s)")}
      <input type="number" min="0" max="1073741824" step="1" disabled={initial.recovery} {...validationProps("bandwidth")} value={(c[key] || 0) / 1024} onChange={(e) => change({ [key]: Number(e.target.value) * 1024 })} />
    </label>)}
    {fieldFeedback("bandwidth")}
    <label className="check"><input type="checkbox" {...validationProps("useProxy")} checked={c.useProxy || false} disabled={initial.recovery} onChange={(e) => change({ useProxy: e.target.checked })} />{t("Use the proxy configured in Preferences")}</label>
    {fieldFeedback("useProxy")}
    {dav && <>
      {(["connectTimeout", "idleTimeout"] as const).map((key) => <label key={key}>{t(key === "connectTimeout" ? "Connection timeout (seconds)" : "HTTP idle timeout (seconds)")}<input type="number" min="0" max="3600" disabled={initial.recovery} {...validationProps(key)} value={c[key] || 0} onChange={(e) => change({ [key]: Number(e.target.value) })} />{fieldFeedback(key)}</label>)}
      {wizard.auth && initial.id && <div className="field-with-help">
        <label><span className="field-label">{t("Alternate HTTPS WebDAV address")}</span><input type="url" {...validationProps("alternateURL")} value={c.alternateURL || ""} disabled={initial.recovery} onChange={(e) => change({ alternateURL: e.target.value })} /></label>
        {fieldFeedback("alternateURL")}
        <FieldHelp label={t("Alternate HTTPS WebDAV address")}>{t("Both addresses must expose the same DAV:resource-id for the selected folder. Fallback is checked only when connecting and never replaces an active mount.")}</FieldHelp>
      </div>}
    </>}
  </>;
}

export function ConnectionOptions({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, validationProps, fieldFeedback, browsers, initial, forgetCredentials, setForgetCredentials } = wizard;
  const sessionSupported = ["webdav", "nextcloud", "owncloud", "drive", "onedrive", "sharepoint"].includes(c.provider);
  return <>
    <fieldset className="option-group"><legend>{t("Files and cache")}</legend><DirectoryCacheOptions wizard={wizard} /></fieldset>
    {supportsNetworkOptions(c.provider) && <fieldset className="option-group"><legend>{t("Network")}</legend><NetworkOptions wizard={wizard} /></fieldset>}
    {wizard.auth && (sessionSupported || wizard.p.oauth && c.authMethod !== "serviceAccount") && <fieldset className="option-group">
      <legend>{t("Sign-in options")}</legend>
      {wizard.p.oauth && c.authMethod !== "serviceAccount" && <>
        <label>{t("Sign-in browser")}<select {...validationProps("browser")} value={c.browser || ""} onChange={(e) => change({ browser: e.target.value })}>{browsers.map((b) => <option key={b.id} value={b.id}>{t(b.name)}</option>)}{c.browser && !browsers.some((b) => b.id === c.browser) && <option value={c.browser}>{t("Selected browser unavailable")}</option>}</select></label>
        {fieldFeedback("browser")}
        <label className="check"><input type="checkbox" checked={c.privateBrowser || false} onChange={(e) => change({ privateBrowser: e.target.checked })} />{t("Open a private sign-in window")}</label>
      </>}
      {sessionSupported && <>
        <div className="option-with-help">
          <label className="check"><input type="checkbox" checked={c.sessionOnly || false} disabled={initial.recovery} onChange={(e) => change({ sessionOnly: e.target.checked, autoMount: e.target.checked ? false : c.autoMount })} />{t("Sign in on every connection (credentials only in memory)")}</label>
          <FieldHelp label={t("Sign in on every connection")}>{t("Session-only sign-in requires a verifiable destination. After disconnecting or reopening, sign in again to the original storage. Browser sessions may still be reused.")}</FieldHelp>
        </div>
        {fieldFeedback("sessionOnly")}
      </>}
      {c.sessionOnly && initial.id && !initial.sessionOnly && <label className="check"><input type="checkbox" required checked={forgetCredentials} onChange={(e) => setForgetCredentials(e.target.checked)} />{t("Remove this connection's saved credentials when setup is confirmed")}</label>}
    </fieldset>}
  </>;
}
