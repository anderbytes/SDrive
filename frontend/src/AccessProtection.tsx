import { useTheme, type Theme } from "./theme";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { api, type AccessPolicy, type AccessStatus } from "./api";
import { useLanguage } from "./i18n";
import { errorMessage } from "./messages";
import { SecretInput } from "./connection/SecretInput";
import { useDialog } from "./useDialog";

const lockLimit = "The lock protects SDrive management controls. Drives that remain connected are still accessible in Explorer.";

export function AccessBoundary({ children }: { children: (showLocked: () => void) => ReactNode }) {
  const { t, setLanguage } = useLanguage();
  const { setTheme } = useTheme();
  const [status, setStatus] = useState<AccessStatus>();
  const [failure, setFailure] = useState("");
  const generation = useRef(0);
  const mounted = useRef(true);
  const refresh = async () => {
    if (!mounted.current) return;
    const request = ++generation.current;
    try {
      const next = await api.accessStatus();
      if (request !== generation.current) return;
      setStatus(next);
      setTheme((next.theme || "system") as Theme);
      setLanguage(next.language === "pt-BR" ? "pt-BR" : "en");
      setFailure("");
    } catch (e) { if (request === generation.current) setFailure(t(errorMessage(e))); }
  };
  const showLocked = () => {
    ++generation.current;
    setStatus((previous) => previous ? { ...previous, locked: true } : undefined);
    void refresh();
  };
  useEffect(() => {
    let active = true;
    mounted.current = true;
    void refresh();
    const unsubscribe = api.onAccessChanged((locked) => {
      if (!active) return;
      ++generation.current;
      if (locked) setStatus((previous) => previous ? { ...previous, locked: true } : undefined);
      void refresh();
    });
    return () => { active = false; mounted.current = false; ++generation.current; unsubscribe(); };
  }, []);
  if (!status) return <main><p role="status">{t("Preparing your drives…")}</p>{failure && <p role="alert">{failure}</p>}<button onClick={() => void refresh()}>{t("Try again")}</button></main>;
  if (status.locked) return <Unlock status={status} onUnlocked={refresh} />;
  return <>{failure && <p role="alert">{failure}</p>}{status.setupPending && <button onClick={async () => { try { await api.end(""); await refresh(); } catch (e) { setFailure(t(errorMessage(e))); } }}>{t("Cancel unfinished setup")}</button>}{status.warning && <p role="status" className="alert warning">{t(status.warning)}</p>}{children(showLocked)}</>;
}

function Unlock({ status, onUnlocked }: { status: AccessStatus; onUnlocked: () => Promise<void> }) {
  const { t } = useLanguage();
  const [pin, setPIN] = useState("");
  const [code, setCode] = useState("");
  const [replacement, setReplacement] = useState("");
  const [recovering, setRecovering] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  return <main className="access-screen"><h1>{t("SDrive is locked")}</h1><p>{t(lockLimit)}</p>
    {status.error && <p role="alert">{t(status.error)}</p>}
    {error && <p role="alert">{error}</p>}
    {replacement ? <RecoveryCode code={replacement} onDone={() => { setReplacement(""); setRecovering(false); setPIN(""); setCode(""); }} /> :
      <form onSubmit={async (event) => {
        event.preventDefault(); setBusy(true); setError("");
        try {
          if (recovering) { setReplacement(await api.recoverAccess(code, pin)); setCode(""); setPIN(""); }
          else { await api.unlockAccess(pin); setPIN(""); await onUnlocked(); }
        } catch (e) { setError(t(errorMessage(e))); setPIN(""); }
        finally { setBusy(false); }
      }}>
        <fieldset disabled={busy || !!status.error}>
          {recovering && <SecretInput label={t("Recovery code")} value={code} required onChange={setCode} />}
          <SecretInput label={t(recovering ? "New PIN" : "Application PIN")} value={pin} required onChange={setPIN} />
          <p>{t("Use 6 to 12 digits. Failed attempts may require a wait, even after restarting.")}</p>
          <button className="primary" type="submit">{t(recovering ? "Reset PIN" : "Unlock")}</button>
          <button type="button" onClick={() => { setRecovering(!recovering); setPIN(""); setCode(""); setError(""); }}>{t(recovering ? "Back to unlock" : "Forgot PIN?")}</button>
        </fieldset>
      </form>}
    <p>{t("If the PIN and recovery code are lost, follow the application lock recovery instructions. Do not delete connection data or cache.")}</p>
    <button disabled={busy} onClick={async () => { try { await api.exit(); } catch (e) { setError(t(errorMessage(e))); } }}>{t("Exit safely")}</button>
  </main>;
}

function RecoveryCode({ code, onDone }: { code: string; onDone: () => void }) {
  const { t } = useLanguage();
  return <section><h2>{t("Save your recovery code")}</h2><p>{t("This code is shown only once. Keep it outside SDrive. It resets your PIN without deleting connections or pending files.")}</p><pre data-sensitive aria-label={t("Recovery code")}>{code}</pre><button type="button" onClick={onDone}>{t("I saved the recovery code")}</button></section>;
}

export function AccessSettings({ onClose }: { onClose: () => void }) {
  const { t } = useLanguage();
  const [status, setStatus] = useState<AccessStatus>();
  const [policy, setPolicy] = useState<AccessPolicy>({ idleMinutes: 15, disconnectIdle: false, reconnect: false });
  const [current, setCurrent] = useState("");
  const [pin, setPIN] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef(true);
  const dialog = useDialog(() => { if (!busy && !code) onClose(); });
  useEffect(() => {
    active.current = true;
    void api.accessStatus().then((next) => { if (active.current) { setStatus(next); setPolicy(next.policy); } }).catch((e) => { if (active.current) setError(t(errorMessage(e))); });
    return () => { active.current = false; };
  }, []);
  return <dialog className="modal" ref={dialog} aria-labelledby="access-settings-title"><h2 id="access-settings-title">{t("Application lock")}</h2><p>{t(lockLimit)}</p>
    {error && <p role="alert">{error}</p>}
    {code ? <RecoveryCode code={code} onDone={onClose} /> : <form onSubmit={async (event) => {
      event.preventDefault();
      if (pin !== confirmation) { setError(t("PIN confirmation does not match.")); return; }
      setBusy(true); setError("");
      try {
        const recovery = await api.configureAccess(current, pin, policy);
        if (!active.current) return;
        setCurrent(""); setPIN(""); setConfirmation("");
        if (recovery) setCode(recovery); else onClose();
      } catch (e) { if (active.current) setError(t(errorMessage(e))); }
      finally { if (active.current) setBusy(false); }
    }}><fieldset disabled={busy || !status}>
      {status?.enabled && <SecretInput label={t("Current PIN")} value={current} required onChange={setCurrent} />}
      <SecretInput label={t("New PIN")} value={pin} required={!status?.enabled} onChange={setPIN} />
      <SecretInput label={t("Confirm new PIN")} value={confirmation} required={!!pin} onChange={setConfirmation} />
      <p>{t("Use 6 to 12 digits. Leave the new PIN empty to keep the existing PIN.")}</p>
      <label>{t("Lock after computer inactivity (minutes; 0 disables)")}<input type="number" min="0" max="1440" required value={policy.idleMinutes} onChange={(e) => {
        const idleMinutes = Number(e.target.value); setPolicy({ ...policy, idleMinutes, disconnectIdle: idleMinutes > 0 && policy.disconnectIdle, reconnect: idleMinutes > 0 && policy.reconnect });
      }} /></label>
      <label className="check"><input type="checkbox" disabled={!policy.idleMinutes} checked={policy.disconnectIdle} onChange={(e) => setPolicy({ ...policy, disconnectIdle: e.target.checked, reconnect: e.target.checked && policy.reconnect })} />{t("Disconnect safely after inactivity")}</label>
      <label className="check"><input type="checkbox" disabled={!policy.disconnectIdle} checked={policy.reconnect} onChange={(e) => setPolicy({ ...policy, reconnect: e.target.checked })} />{t("Reconnect idle-disconnected drives after unlock")}</label>
      <p>{t("Open files, pending uploads or unknown cache state may prevent disconnection. Sign-in or recovery may be required after unlocking.")}</p>
      <div className="actions"><button className="primary" type="submit">{t("Save")}</button>
        {status?.enabled && <button type="button" onClick={async () => { setBusy(true); setError(""); try { await api.disableAccess(current); if (active.current) onClose(); } catch (e) { if (active.current) setError(t(errorMessage(e))); } finally { if (active.current) setBusy(false); } }}>{t("Disable application lock")}</button>}
        <button type="button" onClick={onClose}>{t("Cancel")}</button></div>
    </fieldset></form>}
  </dialog>;
}
