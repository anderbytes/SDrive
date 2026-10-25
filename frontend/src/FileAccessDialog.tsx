import { useEffect, useRef, useState } from "react";
import { api, type FileAccessView } from "./api";
import { useLanguage } from "./i18n";
import { errorMessage } from "./messages";
import { useDialog } from "./useDialog";

export function FileAccessDialog({ connectionID, onClose }: { connectionID: string; onClose: () => void }) {
  const { t } = useLanguage();
  const [result, setResult] = useState<FileAccessView>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef(true);
  const dialog = useDialog(onClose);
  useEffect(() => { active.current = true; return () => { active.current = false; }; }, []);
  const query = async (path: string) => {
    setBusy(true); setError(""); setResult(undefined);
    try {
      const next = await api.inspectFileAccess(connectionID, path);
      if (active.current && next.state !== "cancelled") setResult(next);
    } catch (e) { if (active.current) setError(t(errorMessage(e))); }
    finally { if (active.current) setBusy(false); }
  };
  return <dialog className="modal" ref={dialog} aria-labelledby="file-access-title"><h2 id="file-access-title">{t("Applications using a file")}</h2>
    <p>{t("Select one file in this connected drive. Windows reports applications using that file; reading or writing activity is unknown. This is not a scan of the entire drive.")}</p>
    <button disabled={busy} onClick={() => void query("")}>{t("Select mounted file")}</button>
    {busy && <p role="status">{t("Checking file access…")}</p>}
    {error && <p role="alert">{error}</p>}
    {result && <section><p>{result.path}</p><p>{t("Checked at: {time}", { time: new Date(result.checkedAt).toLocaleTimeString() })}</p>
      {result.state === "reported" ? <>{result.processes.length ? <ul>{result.processes.map((process) => <li key={`${process.pid}:${process.startedAt}`}>{process.name || t("Unknown application")} · PID {process.pid}</li>)}</ul> : <p>{t("Windows reported no applications for this file. Other access may still exist.")}</p>}</> : <p role="status">{t("File access information is unknown. This does not mean the file is unused.")}</p>}
      <button disabled={busy} onClick={() => void query(result.path)}>{t("Refresh")}</button>
    </section>}
    <p>{t("Queued remote uploads are separate from local file access. This query never closes files or stops applications.")}</p>
    <button onClick={onClose}>{t("Close")}</button>
  </dialog>;
}
