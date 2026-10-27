import { useEffect, useId, useRef, useState } from "react";
import { api, type ConnectionView } from "./api";
import { DialogLayout } from "./DialogLayout";
import { DialogNavigation } from "./DialogNavigation";
import { FileAccessInspection } from "./FileAccessInspection";
import { useLanguage } from "./i18n";
import { errorMessage } from "./messages";
import { LoadingStatus } from "./Spinner";
import { useDialog } from "./useDialog";

export function DriveDiagnostics({ connection, busy, onRun, onClose }: {
  connection: ConnectionView;
  busy: boolean;
  onRun: (work: () => Promise<void>, operation: string) => Promise<void>;
  onClose: () => void;
}) {
  const { t, platform } = useLanguage();
  const id = useId();
  const dialog = useDialog(onClose);
  const [section, setSection] = useState("access");
  const [operation, setOperation] = useState("");
  const [results, setResults] = useState<Record<string, { error?: string; message?: string }>>({});
  const active = useRef(true);
  const running = useRef(false);
  useEffect(() => { active.current = true; return () => { active.current = false; }; }, []);
  const run = async (key: string, work: () => Promise<void>, progress: string, success: string) => {
    if (running.current || busy) return;
    running.current = true;
    setOperation(key);
    setResults((previous) => ({ ...previous, [key]: {} }));
    try {
      await onRun(work, progress);
      if (active.current) setResults((previous) => ({ ...previous, [key]: { message: success } }));
    } catch (error) {
      if (active.current) setResults((previous) => ({ ...previous, [key]: { error: t(errorMessage(error)) } }));
    } finally {
      running.current = false;
      if (active.current) setOperation("");
    }
  };
  const unavailable = busy || !!operation;
  const sections = [
    { key: "access", label: "Test access" },
    { key: "refresh", label: "Refresh files in Explorer" },
    { key: "applications", label: "Applications using a file" },
  ];
  return <DialogLayout dialogRef={dialog} titleID={id} title={`${t("Drive diagnostics")} · ${connection.name}`} onClose={onClose}
    navigation={<DialogNavigation label={t("Drive diagnostics")} selected={section} onSelect={setSection}
      items={sections.map((item) => ({ key: item.key, label: t(item.label), id: `${id}-${item.key}`, panel: `${id}-${item.key}-panel` }))} />}
    footer={<button onClick={onClose}>{t("Close")}</button>}>
    {sections.map((item) => <section key={item.key} id={`${id}-${item.key}-panel`} role="tabpanel" aria-labelledby={`${id}-${item.key}`} hidden={section !== item.key}>
      <h3>{t(item.label)}</h3>
      {item.key === "applications" ? <>
        {platform === "linux" && <p>{t("File process inspection is unavailable on Linux.")}</p>}
        {!connection.mounted && <p>{t("Connect the drive to use this tool.")}</p>}
        <FileAccessInspection connectionID={connection.id} disabled={unavailable || !connection.mounted || platform === "linux"} onRun={onRun} />
      </> : <>
        <p>{t(item.key === "access" ? "Listing folders does not verify write permission." : "Refresh directory metadata to see remote changes. Local files and pending uploads are preserved.")}</p>
        {item.key === "refresh" && !connection.mounted && <p>{t("Connect the drive to use this tool.")}</p>}
        <button disabled={unavailable || item.key === "refresh" && !connection.mounted} onClick={() => void run(item.key,
          () => item.key === "access" ? api.test(connection.id) : api.refreshDirectoryCache(connection.id),
          item.key === "access" ? "Validating account…" : "Refreshing directory metadata…",
          item.key === "access" ? "Access test succeeded. Listing folders does not verify write permission." : "Directory metadata refreshed. Reopen the folder in Explorer to see remote changes.")}>{t(item.label)}</button>
        {operation === item.key && <LoadingStatus>{t(item.key === "access" ? "Validating account…" : "Refreshing directory metadata…")}</LoadingStatus>}
        {results[item.key]?.error && <p role="alert">{results[item.key].error}</p>}
        {results[item.key]?.message && <p role="status">{t(results[item.key].message!)}</p>}
      </>}
    </section>)}
  </DialogLayout>;
}
