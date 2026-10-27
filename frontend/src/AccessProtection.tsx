import { useTheme, type Theme } from "./theme";
import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { api, type AccessPolicy, type AccessStatus } from "./api";
import { isLanguage, useLanguage } from "./i18n";
import { errorMessage } from "./messages";
import { SecretInput } from "./connection/SecretInput";
import { useDialog } from "./useDialog";
import { DialogLayout } from "./DialogLayout";
import { LoadingStatus } from "./Spinner";

const lockLimit = "The lock protects SDrive management controls. Drives that remain connected are still accessible in Explorer.";

export function AccessBoundary({ children }: { children: (showLocked: () => void) => ReactNode }) {
  const { t, setLanguage } = useLanguage();
  const { setTheme } = useTheme();
  const [status, setStatus] = useState<AccessStatus>();
  const [failure, setFailure] = useState("");
  const [loading, setLoading] = useState(true);
  const [cancelling, setCancelling] = useState(false);
  const generation = useRef(0);
  const mounted = useRef(true);
  const refresh = async () => {
    if (!mounted.current) return;
    const request = ++generation.current;
    setLoading(true);
    try {
      const next = await api.accessStatus();
      if (request !== generation.current) return;
      setStatus(next);
      setTheme((next.theme || "system") as Theme);
      setLanguage(isLanguage(next.language) ? next.language : "en");
      setFailure("");
    } catch (e) { if (request === generation.current) setFailure(t(errorMessage(e))); }
    finally { if (mounted.current && request === generation.current) setLoading(false); }
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
  if (!status) return <main className="startup-screen">{loading && <LoadingStatus large>{t("Preparing your drives…")}</LoadingStatus>}{failure && <p role="alert">{failure}</p>}{!loading && <button onClick={() => void refresh()}>{t("Try again")}</button>}</main>;
  if (status.locked) return <Unlock status={status} onUnlocked={refresh} />;
  return <>{failure && <p role="alert">{failure}</p>}{cancelling && <LoadingStatus>{t("Operation in progress…")}</LoadingStatus>}{status.setupPending && <button disabled={cancelling} onClick={async () => { setCancelling(true); try { await api.end(""); await refresh(); } catch (e) { setFailure(t(errorMessage(e))); } finally { setCancelling(false); } }}>{t("Cancel unfinished setup")}</button>}{status.warning && <p role="status" className="alert warning">{t(status.warning)}</p>}{children(showLocked)}</>;
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
    {busy && <LoadingStatus>{t("Operation in progress…")}</LoadingStatus>}
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
    <button disabled={busy} onClick={async () => { setBusy(true); try { await api.exit(); } catch (e) { setError(t(errorMessage(e))); } finally { setBusy(false); } }}>{t("Exit safely")}</button>
  </main>;
}

function RecoveryCode({ code, onDone, hideButton = false }: { code: string; onDone: () => void; hideButton?: boolean }) {
  const { t } = useLanguage();
  return <section><h2>{t("Save your recovery code")}</h2><p>{t("This code is shown only once. Keep it outside SDrive. It resets your PIN without deleting connections or pending files.")}</p><pre data-sensitive aria-label={t("Recovery code")}>{code}</pre>{!hideButton && <button type="button" onClick={onDone}>{t("I saved the recovery code")}</button>}</section>;
}

export function AccessSettings({ onClose }: { onClose: () => void }) {
  const { t } = useLanguage();
  const formID = useId();
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
  return <DialogLayout dialogRef={dialog} titleID="access-settings-title" title={t("Application lock")} onClose={onClose} closeDisabled={busy || !!code}
    footer={code ? <button onClick={onClose}>{t("I saved the recovery code")}</button> : <>
      {status?.enabled && <button disabled={busy} type="button" onClick={async () => { setBusy(true); setError(""); try { await api.disableAccess(current); if (active.current) onClose(); } catch (e) { if (active.current) setError(t(errorMessage(e))); } finally { if (active.current) setBusy(false); } }}>{t("Disable application lock")}</button>}
      <button disabled={busy} type="button" onClick={onClose}>{t("Cancel")}</button>
      <button disabled={busy || !status} className="primary" type="submit" form={formID}>{t("Save")}</button>
    </>}><p>{t(lockLimit)}</p>
    {(busy || !status && !error) && <LoadingStatus>{t("Operation in progress…")}</LoadingStatus>}
    {error && <p role="alert">{error}</p>}
    {code ? <RecoveryCode code={code} onDone={onClose} hideButton /> : <form id={formID} onSubmit={async (event) => {
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
    </fieldset></form>}
  </DialogLayout>;
}
