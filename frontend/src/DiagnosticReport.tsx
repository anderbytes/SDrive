import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { DialogLayout } from "./DialogLayout";
import { useDialog } from "./useDialog";
import { useLanguage } from "./i18n";
import { LoadingStatus } from "./Spinner";

export function DiagnosticReport({ onClose }: { onClose: () => void }) {
  const { t } = useLanguage();
  const dialog = useDialog(onClose);
  const [report, setReport] = useState("");
  const [operation, setOperation] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const active = useRef(true);
  const running = useRef(false);
  const run = async (kind: string) => {
    if (running.current) return;
    running.current = true;
    setOperation(kind); setError(""); setNotice("");
    try {
      if (kind === "load") {
        const text = await api.diagnostics();
        if (active.current) setReport(text);
      } else if (kind === "copy") {
        await api.copyDiagnostics(report);
        if (active.current) setNotice("Report copied.");
      } else {
        const saved = await api.exportDiagnosticReport(report);
        if (saved && active.current) setNotice("Report exported.");
      }
    } catch {
      if (active.current) setError(kind === "load" ? "Could not load the diagnostic report. Try again." : kind === "copy" ? "Could not copy the report. Select and copy the text manually." : "Could not export the diagnostic report. Try again.");
    } finally {
      running.current = false;
      if (active.current) setOperation("");
    }
  };
  useEffect(() => {
    active.current = true;
    void run("load");
    return () => { active.current = false; };
  }, []);
  return <DialogLayout dialogRef={dialog} titleID="diagnostic-report-title" title={t("Diagnostic report")} onClose={onClose}
    footer={<>
      <button disabled={!!operation} onClick={() => void run("load")}>{t("Refresh report")}</button>
      <button disabled={!!operation || !report} onClick={() => void run("copy")}>{t("Copy report")}</button>
      <button disabled={!!operation || !report} onClick={() => void run("export")}>{t("Export report")}</button>
      <button onClick={onClose}>{t("Close")}</button>
    </>}>
    <p>{t("Minimal diagnostics are kept locally with fixed limits. Nothing is sent automatically. Review the report before sharing.")}</p>
    {operation && <LoadingStatus>{t(operation === "load" ? "Loading diagnostic report…" : operation === "copy" ? "Copying report…" : "Exporting report…")}</LoadingStatus>}
    {error && <p role="alert">{t(error)}</p>}
    {notice && <p role="status">{t(notice)}</p>}
    {report && <>
      <label htmlFor="diagnostic-report">{t("Diagnostic report (technical text in English)")}</label>
      <textarea id="diagnostic-report" className="diagnostic-report" readOnly value={report} spellCheck={false} />
    </>}
  </DialogLayout>;
}
