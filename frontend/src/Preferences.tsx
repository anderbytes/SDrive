import { useEffect, useId, useState } from "react";
import { flushSync } from "react-dom";
import { api, type Snapshot, type ProxyConfig, type Settings } from "./api";
import { languages, useLanguage, type Language } from "./i18n";
import { errorMessage as message, type LocalizedMessage } from "./messages";
import { SecretInput } from "./connection/SecretInput";
import { ConfigurationPortability } from "./ConfigurationPortability";
import { useDialog } from "./useDialog";
import { DialogLayout } from "./DialogLayout";
import { DialogNavigation } from "./DialogNavigation";
import { useTheme, type Theme } from "./theme";
import { LoadingStatus } from "./Spinner";

const tabs = ["General", "Appearance", "Cache", "Network", "Configuration"] as const;

export function Preferences({
  data,
  onRefresh,
  onClose,
}: {
  data: Snapshot;
  onRefresh: () => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const helpID = useId();
  const [tab, setTab] = useState<typeof tabs[number]>("General");
  const { setTheme, setPreview } = useTheme();
  useEffect(() => () => setPreview(undefined), [setPreview]);
  const [settings, setSettings] = useState({ ...data.settings, theme: data.settings.theme || "system" });
  const [cachePath, setCachePath] = useState(data.cacheLocation);
  const [effectiveCache, setEffectiveCache] = useState(data.cacheLocation);
  const [cacheNotice, setCacheNotice] = useState("");
  const [defaults, setDefaults] = useState<Settings>();
  useEffect(() => { let active = true; void api.preferenceDefaults().then((value) => { if (active) setDefaults(value); }).catch((e) => { if (active) setError(message(e)); }); return () => { active = false; }; }, []);
  const [proxy, setProxy] = useState<ProxyConfig>({ address: "", username: "", password: "", hasPassword: false });
  const [proxyReady, setProxyReady] = useState(false);
  const [proxyDirty, setProxyDirty] = useState(false);
  useEffect(() => { let active = true; void api.proxy().then((p) => { if (active) { setProxy(p); setProxyReady(true); } }).catch((e) => { if (active) setError(message(e)); }); return () => { active = false; }; }, []);

  const [error, setError] = useState<string | LocalizedMessage>("");
  const [busy, setBusy] = useState(false);
  const dialog = useDialog(() => {
    if (!busy) onClose();
  });
  return (
    <DialogLayout dialogRef={dialog} titleID="preferences-title" title={t("Preferences")} onClose={onClose} closeDisabled={busy}
      navigation={<DialogNavigation label={t("Preferences categories")} selected={tab} onSelect={(key) => setTab(key as typeof tab)}
        items={tabs.map((name) => ({ key: name, label: t(name), id: `${helpID}-${name}-tab`, panel: `${helpID}-${name}-panel`, disabled: busy }))} />}
      footer={<><button type="button" disabled={busy} onClick={onClose}>{t("Cancel")}</button><button form={`${helpID}-form`} className="primary" disabled={busy}>{t("Save")}</button></>}>
        {(busy || !error && (tab === "Cache" && !defaults || tab === "Network" && !proxyReady)) && <LoadingStatus>{t("Operation in progress…")}</LoadingStatus>}
        {error && (
          <div role="alert" className="alert error">
            {t(error)}
          </div>
        )}
        <form id={`${helpID}-form`} noValidate
          onSubmit={async (e) => {
            e.preventDefault();
            if (busy) return;
            const invalid = Array.from(e.currentTarget.elements).find((element) => element instanceof HTMLInputElement && !element.validity.valid) as HTMLInputElement | undefined;
            if (invalid) {
              const panel = invalid.closest<HTMLElement>("[data-preference-tab]");
              if (panel) flushSync(() => setTab(panel.dataset.preferenceTab as typeof tab));
              invalid.focus(); invalid.reportValidity(); return;
            }
            setBusy(true);
            try {
              if (proxyDirty) await api.saveProxy(proxy);
              await api.settings(settings);
              setTheme(settings.theme);
              setPreview(undefined);
              onClose();
            } catch (e) {
              setError(message(e));
            } finally {
              setBusy(false);
            }
          }}
        >
          <div id={`${helpID}-General-panel`} role="tabpanel" aria-labelledby={`${helpID}-General-tab`} data-preference-tab="General" hidden={tab !== "General"}>
            <label>
              {t("Language")}
              <select
                value={settings.language || "en"}
                onChange={(e) =>
                  setSettings((s) => ({
                    ...s,
                    language: e.target.value as Language,
                  }))
                }
              >
                {Object.entries(languages).map(([code, name]) => <option key={code} value={code}>{name}</option>)}
              </select>
            </label>
            <label className="check">
              <input
                type="checkbox"
                checked={settings.autoStart}
                onChange={(e) =>
                  setSettings((s) => ({ ...s, autoStart: e.target.checked }))
                }
              />
              {t("Start with Windows, in the tray")}
            </label>
          </div>
          <div id={`${helpID}-Appearance-panel`} role="tabpanel" aria-labelledby={`${helpID}-Appearance-tab`} data-preference-tab="Appearance" hidden={tab !== "Appearance"}>
            <label>{t("Theme")}<select aria-label={t("Theme")} disabled={busy} value={settings.theme} onChange={(e) => {
              const theme = e.target.value as Theme; setSettings({ ...settings, theme }); setPreview(theme);
            }}><option value="system">{t("System")}</option><option value="light">{t("Light")}</option><option value="dark">{t("Dark")}</option></select></label>
            <p>{t("System follows your Windows theme. Save to keep the preview; Cancel restores the saved theme.")}</p>
            <label className="check"><input type="checkbox" disabled={data.platform === "linux"} checked={settings.showServiceIcons || false} onChange={(e) => setSettings({ ...settings, showServiceIcons: e.target.checked })} />{t("Show service icons for drive letters")}</label>
            {data.platform === "linux" && <p>{t("Drive-letter icons are available only on Windows.")}</p>}
            <p>{t("Service icons are local monograms for SDrive drive letters. Existing customization takes precedence; mounted folders keep their icons.")}</p>
          </div>
          <div id={`${helpID}-Cache-panel`} role="tabpanel" aria-labelledby={`${helpID}-Cache-tab`} data-preference-tab="Cache" hidden={tab !== "Cache"}>
            <div className="field-action">
            <label>
              {t("Cache target per connection (GiB)")}
              <input
                type="number"
                min="1"
                max="1024"
                aria-describedby={`${helpID}-cache-help`}
                value={settings.cacheGiB}
                onChange={(e) =>
                  setSettings((s) => ({ ...s, cacheGiB: Number(e.target.value) }))
                }
              />
            </label>
            <button type="button" disabled={busy || !defaults} onClick={() => setSettings({ ...settings, cacheGiB: defaults!.cacheGiB })}>{t("Restore cache size")}</button>
            </div>
            <p id={`${helpID}-cache-help`}>
              {t(
                "Applied on the next mount. Open files and pending uploads may exceed the target. SDrive reserves 5 GiB of free space before connecting.",
              )}
            </p>
            <label>{t("Cache retention after last access (hours)")}<input type="number" min="1" max="4320" step="1" value={settings.cacheMaxAgeHours ?? 24} onChange={(e) => setSettings({ ...settings, cacheMaxAgeHours: Number(e.target.value) })} /></label>
            <small>{t("Retention accepts 1–4320 hours (180 days). It controls eviction after last access, not offline availability; pending uploads remain protected.")}</small>
            <label>{t("Upload delay after last use (seconds)")}<input type="number" min="0" max="3600" step="1" value={settings.writeBackSeconds ?? 5} onChange={(e) => setSettings({ ...settings, writeBackSeconds: Number(e.target.value) })} /></label>
            <button type="button" disabled={busy || !defaults} onClick={() => setSettings({ ...settings, cacheMaxAgeHours: defaults!.cacheMaxAgeHours, writeBackSeconds: defaults!.writeBackSeconds })}>{t("Restore cache timing")}</button>
            <p>{t("Timing changes apply on the next connection. Saving a file does not confirm its upload. Open files and pending uploads remain protected regardless of cache retention.")}</p>
            <fieldset disabled={busy}><legend>{t("Local cache location")}</legend>
              <p>{t("Current cache: {path}", { path: effectiveCache })}</p>
              <div className="field-action"><label>{t("New empty cache directory")}<input type="text" value={cachePath} onChange={(e) => setCachePath(e.target.value)} /></label>
              <button type="button" onClick={async () => {
                setBusy(true); setError("");
                try { const path = await api.selectDirectory(); if (path) setCachePath(path); } catch (e) { setError(message(e)); } finally { setBusy(false); }
              }}>{t("Choose directory")}</button></div>
              <button type="button" disabled={!cachePath || cachePath === effectiveCache} onClick={async () => {
                setBusy(true); setError(""); setCacheNotice("");
                try { await api.changeCacheLocation(cachePath); setEffectiveCache(cachePath); setCacheNotice("Cache location changed. The previous cache was preserved."); } catch (e) { setError(message(e)); } finally { setBusy(false); }
              }}>{t("Change cache location")}</button>
              <p>{t("Disconnect all drives and resolve pending uploads first. The new directory must be empty and outside this profile. The old cache is preserved; files are not migrated or deleted.")}</p>
              <button type="button" onClick={async () => {
                setBusy(true); setError(""); setCacheNotice("");
                try { const path = await api.restoreDefaultCacheLocation(); setCachePath(path); setEffectiveCache(path); setCacheNotice("Cache location changed. The previous cache was preserved."); } catch (e) { setError(message(e)); } finally { setBusy(false); }
              }}>{t("Restore default cache location")}</button>
              <p>{t("Restoring the default location applies immediately, independently of Save or Cancel. Existing files in the default directory prevent restoration; both caches are preserved.")}</p>
              {cacheNotice && <p role="status">{t(cacheNotice)}</p>}
            </fieldset>
            <p>
              {t(
                "Credentials are encrypted. The cache contains local files accessible to your Windows user.",
              )}
            </p>
          </div>
          <div id={`${helpID}-Network-panel`} role="tabpanel" aria-labelledby={`${helpID}-Network-tab`} data-preference-tab="Network" hidden={tab !== "Network"}>
            <fieldset disabled={!proxyReady || busy}><legend>{t("Connection proxy")}</legend>
              {(["address", "username"] as const).map((key) => <label key={key}>{t(key === "address" ? "Proxy HTTP/HTTPS/SOCKS5 address" : "Proxy username")}
                <input type="text" autoComplete="off" value={proxy[key]} onChange={(e) => { setProxy({ ...proxy, [key]: e.target.value }); setProxyDirty(true); }} />
              </label>)}
              <SecretInput label={t("Proxy password")} value={proxy.password} required={false} onChange={(value) => { setProxy({ ...proxy, password: value }); setProxyDirty(true); }} />
              <button type="button" onClick={() => { setProxy({ address: "", username: "", password: "", hasPassword: false }); setProxyDirty(true); }}>{t("Restore proxy defaults")}</button>
              <p>{t("Save removes the proxy address and saved credentials. Connections that require a proxy keep that choice and cannot connect until a proxy is configured.")}</p>
              {proxy.hasPassword && <p>{t("A proxy password is saved. Leave blank to keep it for the same address and username.")}</p>}
              <p>{t("Choose Use proxy separately for each connection. Changes apply on the next connection; the local engine API always stays direct.")}</p>
              <p>{t("SOCKS5 resolves destination names through the proxy. Browser sign-in uses the browser's own network settings.")}</p>
            </fieldset>
          </div>
        </form>
          <div id={`${helpID}-Configuration-panel`} role="tabpanel" aria-labelledby={`${helpID}-Configuration-tab`} data-preference-tab="Configuration" hidden={tab !== "Configuration"}>
            <ConfigurationPortability data={data} onRefresh={async () => { onClose(); await onRefresh(); }} />
          </div>
    </DialogLayout>
  );
}
