import { Preferences } from "./Preferences";
import { ThemeProvider, useTheme } from "./theme";
import { errorMessage as message, type LocalizedMessage } from "./messages";
import { FileAccessDialog } from "./FileAccessDialog";
import { DriveDetails, type DriveDetailSelection } from "./DriveDetails";
import { formatBytes as bytes } from "./formatBytes";
import { AccessBoundary, AccessSettings } from "./AccessProtection";
import { ConnectionEditor } from "./connection/ConnectionEditor";
import { useDialog } from "./useDialog";
import { LanguageProvider, isLanguage, useLanguage } from "./i18n";
import { useEffect, useRef, useState } from "react";
import {
  api,
  type AboutInfo,
  type Connection,
  type ConnectionView,
  type Provider,
  type Snapshot,
  type ImportDraft,
} from "./api";

const operationText = {
  "": "",
  validating: "Validating account…",
  mounting: "Mounting drive…",
  checkingUploads: "Checking pending uploads…",
  waitingUploads: "Waiting for uploads…",
  unmounting: "Unmounting drive…",
};
const blank = (letter: string): Connection => ({
  id: "",
  remote: "",
  name: "",
  provider: "webdav",
  path: "",
  letter,
  readOnly: false,
  autoMount: false,
  recovery: false,
  targetHash: "",
});

export function App() {
  return (
    <ThemeProvider>
      <LanguageProvider>
        <AccessBoundary>{(showLocked) => <AppContent onLocked={showLocked} />}</AccessBoundary>
      </LanguageProvider>
    </ThemeProvider>
  );
}

function AppContent({ onLocked }: { onLocked: () => void }) {
  const { t, setLanguage } = useLanguage();
  const { setTheme } = useTheme();
  const [data, setData] = useState<Snapshot>();
  const [error, setError] = useState<string | LocalizedMessage>("");
  const [notice, setNotice] = useState("");
  const [editor, setEditor] = useState<{
    connection: Connection;
    auth: boolean;
    importDraft?: ImportDraft;
  }>();
  const [working, setWorking] = useState<Record<string, string>>({});
  const [driveDetail, setDriveDetail] = useState<DriveDetailSelection>();
  const [settings, setSettings] = useState(false);
  const [accessSettings, setAccessSettings] = useState(false);
  const [about, setAbout] = useState(false);
  const [restoreConfirm, setRestoreConfirm] = useState(false);
  const [announcement, setAnnouncement] = useState("");
  const previousStates = useRef<Map<string, string>>(undefined);
  useEffect(() => {
    if (!data || data.settingsError) return;
    const states = new Map(data.connections.map((c) => [c.id,
      c.pending === null ? "Pending uploads unknown" : c.mounted ? "Connected" : c.recovery ? "Recovery required" : "Disconnected",
    ]));
    if (previousStates.current) {
      const changes = data.connections.filter((c) => states.get(c.id) !== previousStates.current!.get(c.id));
      if (changes.length) setAnnouncement(changes.map((c) => t("{name}: {state}", { name: c.name, state: t(states.get(c.id)!) })).join(". "));
    }
    previousStates.current = states;
  }, [data, t]);
  const snapshotRequest = useRef(0);
  const snapshotActive = useRef(true);
  const refresh = async () => {
    if (!snapshotActive.current) return;
    const request = ++snapshotRequest.current;
    try {
      const snapshot = await api.snapshot();
      if (!snapshotActive.current || request !== snapshotRequest.current) return;
      if (snapshot.access.locked) { onLocked(); return; }
      setData(snapshot);
      setTheme(snapshot.settings.theme || "system");
      setLanguage(isLanguage(snapshot.settings.language) ? snapshot.settings.language : "en");
    } catch (e) {
      if (snapshotActive.current && request === snapshotRequest.current) setError(message(e));
    }
  };
  useEffect(() => {
    snapshotActive.current = true;
    let active = true;
    let visible = true;
    let generation = 0;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (!active || !visible) return;
      const current = generation;
      await refresh();
      if (active && visible && current === generation) timer = setTimeout(poll, 3000);
    };
    const visibility = (next: boolean) => {
      if (!active) return;
      ++generation;
      ++snapshotRequest.current;
      clearTimeout(timer);
      visible = next;
      if (visible) void poll();
    };
    const unsubscribe = api.onDesktopVisibility(visibility);
    const initialGeneration = generation;
    void api.desktopVisible().then((next) => {
      if (active && generation === initialGeneration && next !== visible) visibility(next);
    }).catch(() => {});
    void poll();
    return () => {
      active = false;
      snapshotActive.current = false;
      ++snapshotRequest.current;
      clearTimeout(timer);
      unsubscribe();
    };
  }, []);
  const run = async (
    key: string,
    action: () => Promise<void>,
    success = "",
    operation = "Operation in progress…",
  ) => {
    ++snapshotRequest.current;
    setWorking((v) => ({ ...v, [key]: operation }));
    setError("");
    setNotice("");
    try {
      await action();
      if (!snapshotActive.current) return;
      ++snapshotRequest.current;
      setNotice(success);
      await refresh();
    } catch (e) {
      if (!snapshotActive.current) return;
      ++snapshotRequest.current;
      setError(message(e));
    } finally {
      if (snapshotActive.current) setWorking((v) => {
        const next = { ...v };
        delete next[key];
        return next;
      });
    }
  };
  return (
    <div className="shell">
      {accessSettings && <AccessSettings onClose={() => { setAccessSettings(false); void refresh(); }} />}
      <aside>
        <div className="brand">
          <span className="logo">S</span>
          <div>
            {" "}
            SDrive<small>{t("YOUR DRIVE. YOUR CODE.")}</small>
          </div>
        </div>
        <div className="nav-active">{t("▣   My drives")}</div>
        <button
          className="nav"
          disabled={!data || !!data.settingsError}
          onClick={() => setSettings(true)}
        >
          {t("⚙   Preferences")}
        </button>
        <button className="nav" onClick={() => setAccessSettings(true)}>{t("Application lock")}</button>
        {data?.access?.enabled && <button className="nav" onClick={() => void run("lock", api.lock)}>{t("Lock SDrive")}</button>}
        <footer>
          {t("Open source · GPLv3")}
          <br /> Go + rclone <br />
          <button className="nav" onClick={() => setAbout(true)}>{t("About SDrive")}</button>
          <button
            className="nav"
            disabled={!!working["exit"]}
            onClick={() => void run("exit", api.exit)}
          >
            {t(data?.settingsError ? "Close SDrive" : "Exit safely")}
          </button>
        </footer>
      </aside>
      <main>
        <header>
          <div>
            <div className="eyebrow">{t("CONNECTED STORAGE")}</div>
            <h1>{t("My drives")}</h1>
            <p>{t("Your cloud files, right in Explorer.")}</p>
          </div>
          <button
            className="primary"
            disabled={
              !data || !!data.engineError || !!data.settingsError || data.recoveryHold
            }
            onClick={() =>
              setEditor({
                connection: blank(
                  data!.letters.includes("S") ? "S" : data!.letters[0] || "S",
                ),
                auth: true,
              })
            }
          >
            {t("+ New connection")}
          </button>
        </header>
        {data && !data.settingsError && !data.letters.length && (
          <p role="status" className="alert warning">{t("No drive letters are available. Choose a new local subfolder or free a drive letter.")}</p>
        )}
        {error && !restoreConfirm && (
          <div role="alert" className="alert error">
            {t(error)}
            <button
              aria-label={t("Dismiss notification")}
              onClick={() => setError("")}
            >
              {" "}
              ×{" "}
            </button>
          </div>
        )}
        {notice && (
          <div role="status" className="alert success">
            {t(notice)}
          </div>
        )}
        {!data && <p role="status">{t("Preparing your drives…")}</p>}
        {data?.settingsError && (
          <section className="alert error" role="alert">
            <div>
              <h2>{t("Settings recovery")}</h2>
              <p>{t(data.settingsError)}</p>
              <p>
                {t("Connections, credentials and cache will not be changed while settings are unavailable.")}
              </p>
              {data.backupAvailable ? (
                <button onClick={() => {
                  setError("");
                  setRestoreConfirm(true);
                }}>
                  {t("Restore saved backup")}
                </button>
              ) : (
                <p>
                  {t("No compatible backup is available. Preserve the data directory for manual recovery.")}
                </p>
              )}
            </div>
          </section>
        )}
        {data?.recoveryHold && !data.settingsError && (
          <section className="alert warning" role="status">
            <div>
              <strong>{t("Settings recovery is protected")}</strong>
              <p>
                {t("Automatic connection and credential cleanup are suspended. Recover and disconnect restored drives, then verify recovery. Unknown credentials and cache require manual recovery.")}
              </p>
              <button
                disabled={!!working["verify-recovery"]}
                onClick={() => void run("verify-recovery", api.verifyRecovery, "Settings recovery verified.")}
              >
                {t("Verify recovery")}
              </button>
            </div>
          </section>
        )}
        {data?.maintenanceError && !data.settingsError && (
          <div className="alert warning" role="alert">
            {t(data.maintenanceError)}
            <button
              disabled={data.recoveryHold || !!working["cleanup"]}
              onClick={() => void run("cleanup", api.retryCleanup, "Credential cleanup completed.")}
            >
              {t("Retry cleanup")}
            </button>
          </div>
        )}
        {data?.desktopWarning && <p role="status" className="alert warning">{data.desktopWarning.split("\n").map((warning) => t(warning)).join(" ")}</p>}
        {data?.startupWarning && !data.settingsError && (
          <p className="alert warning" role="alert">{t(data.startupWarning)}</p>
        )}
        {data?.engineError && !data.settingsError && (
          <div role="alert" className="alert error">
            {t(data.engineError)}
            <button
              disabled={!!working["restart"]}
              onClick={() =>
                void run(
                  "restart",
                  api.restart,
                  "Engine restarted. Recover pending connections.",
                )
              }
            >
              {t("Restart engine")}
            </button>
          </div>
        )}
        {data && !data.settingsError && !data.winFsp && (
          <div className="alert warning">
            <div>
              <strong>{t("Installation needs repair")}</strong>
              <p>
                {t(
                  "Run the SDrive installer again. It prepares the required components and may request administrator permission.",
                )}
              </p>
            </div>
          </div>
        )}
        {data && !data.settingsError && (
          <div className="summary">
            <div>
              <strong>{data.connections.length}</strong> {t("connections")}
            </div>
            <div>
              <span className="dot" />
              <strong>
                {data.connections.filter((c) => c.mounted).length}
              </strong>{" "}
              {t("connected")}
            </div>
            <div>
              <strong>
                {data.connections.some((c) => c.pending === null)
                  ? t("{count} known; others unknown", {
                      count: data.connections.reduce((n, c) => n + (c.pending ?? 0), 0),
                    })
                  : data.connections.reduce((n, c) => n + (c.pending ?? 0), 0)}
              </strong>{" "}
              {t("pending uploads")}
            </div>
          </div>
        )}
        {data && !data.settingsError && (
          <p className="hint">{t("Cache target: {count} GiB per connection", { count: data.settings.cacheGiB })}
            {" · "}{t("Free on cache disk: {size}", { size: bytes(data.freeBytes) })}
            {" · "}{t("Pending files are preserved even when the target is exceeded.")}
          </p>
        )}
        <p className="sr-only" aria-live="polite" aria-atomic="true">{announcement}</p>
        <section aria-label={t("Drives")} className="drives">
          {data?.importDrafts?.map((draft) => <article className="drive" key={draft.id}>
            <div className="drive-info"><h2>{draft.definition.name}</h2>
              <p>{data.providers.find((p) => p.id === draft.definition.provider)?.name} · {draft.definition.mountFolder || `${draft.definition.letter}:`}</p>
              <p role="status">{t("Authentication required for imported configuration")}</p>
              <p>{t("Supply application credentials and authenticate again. No credentials or pending files were imported.")}</p>
            </div>
            <div className="actions">
              <button disabled={!!data.settingsError || data.recoveryHold || !!data.engineError} onClick={() => setEditor({ connection: { ...blank(draft.definition.letter), ...draft.definition }, auth: true, importDraft: draft })}>{t("Authenticate")}</button>
              <button disabled={!!data.settingsError || data.recoveryHold || !!working[draft.id]} onClick={() => void run(draft.id, () => api.removeImportDraft(draft.id))}>{t("Remove draft")}</button>
            </div>
          </article>)}
          {data?.connections.map((c) => (
            <Drive
              key={c.id}
              connection={c}
              detailSelection={driveDetail}
              onDetailSelect={setDriveDetail}
              provider={data.providers.find((p) => p.id === c.provider)!}
              busy={!!working[c.id] || c.busy}
              operation={operationText[c.operation] || working[c.id] || ""}
              onAction={(action, operation, success) => void run(c.id, () => action(c.id), success, operation)}
              onEdit={(auth) => setEditor({ connection: c, auth })}
            />
          ))}
        </section>
        {data && !data.settingsError && !data.recoveryHold && data.connections.length === 0 && !data.importDrafts?.length && (
          <section className="empty">
            <div className="empty-icon">▱</div>
            <h2>{t("One letter. All your files.")}</h2>
            <p>
              {t(
                "Connect an account or server and choose how it appears in Windows.",
              )}
            </p>
            <button
              disabled={!!data.engineError}
              onClick={() =>
                setEditor({
                  connection: blank(
                    data.letters.includes("S") ? "S" : data.letters[0] || "S",
                  ),
                  auth: true,
                })
              }
            >
              {t("Add my first connection")}
            </button>
            <small>
              {t("18 connection types · Credentials protected by Windows")}
            </small>
          </section>
        )}
        <div className="hint">
          {t(
            "Closing the window keeps your drives active in the tray. Wait for uploads before exiting.",
          )}
        </div>
      </main>
      {editor && data && (
        <ConnectionEditor
          importDraft={editor.importDraft}
          initial={editor.connection}
          auth={editor.auth}
          data={data}
          onRefresh={refresh}
          onClose={() => {
            setEditor(undefined);
            void refresh();
          }}
        />
      )}
      {settings && data && (
        <Preferences
          data={data}
          onRefresh={refresh}
          onClose={() => {
            setSettings(false);
            void refresh();
          }}
        />
      )}
      {restoreConfirm && (
        <RestoreSettingsDialog
          busy={!!working["restore"]}
          error={error}
          onClose={() => setRestoreConfirm(false)}
          onConfirm={() => void run(
            "restore",
            async () => {
              await api.restoreSettings();
              setRestoreConfirm(false);
            },
            "Settings restored. Review recovery before connecting drives.",
          )}
        />
      )}
      {about && <AboutDialog onClose={() => setAbout(false)} />}
    </div>
  );
}

function AboutDialog({ onClose }: { onClose: () => void }) {
  const { t } = useLanguage();
  const dialog = useDialog(onClose);
  const [info, setInfo] = useState<AboutInfo>();
  const [error, setError] = useState<string | LocalizedMessage>("");
  const [report, setReport] = useState("");
  const [loadingReport, setLoadingReport] = useState(false);
  const [copying, setCopying] = useState(false);
  const [reportError, setReportError] = useState<string | LocalizedMessage>("");
  const [copied, setCopied] = useState(false);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    let current = true;
    void api.about().then((value) => { if (current) setInfo(value); })
      .catch((e) => { if (current) setError(message(e)); });
    return () => { current = false; active.current = false; };
  }, []);
  const loadReport = async () => {
    setLoadingReport(true);
    setReportError("");
    setCopied(false);
    try {
      const text = await api.diagnostics();
      if (active.current) setReport(text);
    } catch {
      if (active.current) setReportError("Could not load the diagnostic report. Try again.");
    } finally {
      if (active.current) setLoadingReport(false);
    }
  };
  const copyReport = async () => {
    setCopying(true);
    setReportError("");
    setCopied(false);
    try {
      await api.copyDiagnostics(report);
      if (active.current) setCopied(true);
    } catch {
      if (active.current) setReportError("Could not copy the report. Select and copy the text manually.");
    } finally {
      if (active.current) setCopying(false);
    }
  };
  return <dialog ref={dialog} className="modal small" role="dialog" aria-modal="true" aria-labelledby="about-title">
    <h2 id="about-title">{t("About SDrive")}</h2>
    <p>{t("Open source · GPLv3")}</p>
    {error && <p role="alert" className="alert error">{t(error)}</p>}
    {!info && !error && <p role="status">{t("Loading component versions…")}</p>}
    {info && <dl className="about-versions">{([
      ["SDrive", info.appVersion], ["rclone (running)", info.rcloneVersion],
      ["Windows", info.windowsVersion], ["WinFsp (installed)", info.winFspVersion],
      ["WebView2 (installed)", info.webView2Version], ["Architecture", info.architecture],
    ]).map(([label, value]) => <div key={label}><dt>{t(label)}</dt><dd>{value || t("Unavailable or not installed")}</dd></div>)}</dl>}
    <p>{t("Updates are installed manually. Use Exit safely and wait for uploads before running an installer.")}</p>
    <p>{t("Minimal diagnostics are kept locally with fixed limits. Nothing is sent automatically. Review the report before sharing.")}</p>
    <button disabled={loadingReport || copying} onClick={() => void loadReport()}>{t(loadingReport ? "Loading diagnostic report…" : "View diagnostics")}</button>
    {reportError && <p role="alert" className="alert error">{t(reportError)}</p>}
    {report && <>
      <label htmlFor="diagnostic-report">{t("Diagnostic report (technical text in English)")}</label>
      <textarea id="diagnostic-report" className="diagnostic-report" readOnly value={report} spellCheck={false} />
      <button disabled={copying || loadingReport} onClick={() => void copyReport()}>{t("Copy report")}</button>
    </>}
    {copied && <p role="status">{t("Report copied.")}</p>}
    <div className="modal-actions"><button onClick={onClose}>{t("Close")}</button></div>
  </dialog>;
}

function RestoreSettingsDialog({
  busy, error, onClose, onConfirm,
}: {
  busy: boolean;
  error: string | LocalizedMessage;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const { t } = useLanguage();
  const dialog = useDialog(() => {
    if (!busy) onClose();
  });
  return (
    <dialog
        ref={dialog}
        className="modal small"
        role="dialog"
        aria-modal="true"
        aria-labelledby="restore-title"
      >
        <h2 id="restore-title">{t("Restore settings backup?")}</h2>
        {error && <div className="alert error" role="alert">{t(error)}</div>}
        <p>
          {t("The backup may omit newer connections. Damaged settings will be archived; credentials and cache will be preserved. Automatic connection and cleanup will remain suspended until recovery is verified.")}
        </p>
        <div className="modal-actions">
          <button disabled={busy} onClick={onClose}>{t("Cancel")}</button>
          <button disabled={busy} className="primary" onClick={onConfirm}>{t("Confirm restoration")}</button>
        </div>
    </dialog>
  );
}

function Drive({
  connection: c,
  provider: p,
  detailSelection,
  onDetailSelect,
  busy,
  operation,
  onAction,
  onEdit,
}: {
  connection: ConnectionView;
  provider: Provider;
  detailSelection: DriveDetailSelection | undefined;
  onDetailSelect: (selection: DriveDetailSelection | undefined) => void;
  busy: boolean;
  operation: string;
  onAction: (action: (id: string) => Promise<void>, operation?: string, success?: string) => void;
  onEdit: (auth: boolean) => void;
}) {
  const { t } = useLanguage();
  const [fileAccess, setFileAccess] = useState(false);
  const menu = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const outside = (event: PointerEvent) => {
      if (menu.current && !menu.current.contains(event.target as Node)) menu.current.open = false;
    };
    document.addEventListener("pointerdown", outside);
    return () => document.removeEventListener("pointerdown", outside);
  }, []);
  const closeMenu = () => {
    if (!menu.current) return;
    menu.current.open = false;
    menu.current.querySelector("summary")?.focus();
  };
  return (
    <article className="drive">
      {fileAccess && <FileAccessDialog connectionID={c.id} onClose={() => setFileAccess(false)} />}
      <div className={`letter ${c.mounted ? "connected" : ""}`}>
        {c.mountFolder ? "▱" : <>{c.letter}<span>:</span></>}
      </div>
      <div className="drive-info">
        <h2>{c.name}</h2>
        {c.mountFolder && <p>{c.mountFolder}</p>}
        <p>
          {t(p.name)}
          {p.experimental ? ` · ${t("Experimental")}` : ""}
        </p>
        <div className="state">
          {busy
            ? t(operation || "Operation in progress…")
            : c.pending === null
              ? t("Pending uploads unknown")
              : c.mounted
                ? c.pending
                  ? t("{count} pending upload(s)", { count: c.pending })
                  : t("Connected")
                : c.recovery
                  ? t("Recovery required")
                  : t("Disconnected")}
        </div>
        {c.recovery && !c.mounted && (busy || c.pending === null) && <p className="inline-error">{t("Recovery required")}</p>}
        {!!c.error && <p className="inline-error">{t("Connection needs attention.")}</p>}
        {(!!c.cache.error || !!c.uploads?.cacheErrors) && <p className="inline-error">{t("Local cache needs attention.")}</p>}
        {c.mounted && c.transportSecurity?.state !== "encrypted" && <p className="inline-error">{t(c.transportSecurity?.state === "unencrypted" ? "Traffic unencrypted" : "Traffic encryption could not be verified")}</p>}
        <DriveDetails connection={c} provider={p} selection={detailSelection} onSelect={onDetailSelect} />
      </div>
      <div className="actions">
        <button
          disabled={busy}
          className={c.mounted ? "" : "primary"}
          onClick={() => c.sessionOnly && !c.mounted ? onEdit(true) : onAction(c.mounted ? api.unmount : api.mount, c.mounted ? "Checking pending uploads…" : "Validating account…")}
        >
          {c.mounted
            ? t("Disconnect")
            : c.recovery
              ? t("Recover")
              : t("Connect")}
        </button>
        {c.mounted && (
          <button disabled={busy} onClick={() => onAction(api.open)}>
            {" "}
            Explorer ↗{" "}
          </button>
        )}
        <details ref={menu}
          onBlur={(event) => {
            if (!event.currentTarget.contains(event.relatedTarget as Node | null)) event.currentTarget.open = false;
          }}
          onKeyDown={(event) => {
            if (event.key === "Escape" && event.currentTarget.open) {
              event.preventDefault();
              event.stopPropagation();
              closeMenu();
            }
          }}>
          <summary aria-label={t("More actions for {name}", { name: c.name })}>
            •••
          </summary>
          <div className="dropdown" onClickCapture={(event) => {
            if ((event.target as HTMLElement).closest("button:not(:disabled)")) closeMenu();
          }}>
            <button disabled={busy || !c.mounted} onClick={() => { closeMenu(); setFileAccess(true); }}>{t("Applications using a file")}</button>
          <button disabled={busy || !c.mounted} onClick={() => onAction(api.refreshDirectoryCache, "Refreshing directory metadata…", "Directory metadata refreshed. Reopen the folder in Explorer to see remote changes.")}>{t("Refresh files in Explorer")}</button>
            <button disabled={busy} onClick={() => onAction(api.test, "Validating account…", "Access test succeeded. Listing folders does not verify write permission.")}>
              {t("Test access")}
            </button>
            <button
              disabled={busy || c.mounted || c.recovery}
              onClick={() => onEdit(false)}
            >
              {t("Edit drive")}
            </button>
            <button disabled={busy || c.mounted} onClick={() => onEdit(true)}>
              {t("Reconfigure account")}
            </button>
            <button
              disabled={busy || c.mounted || c.recovery}
              className="danger"
              onClick={() => {
                if (
                  window.confirm(
                    t(
                      "Remove connection {name}? Remote files will be preserved.",
                      { name: c.name },
                    ),
                  )
                )
                  onAction(api.remove);
              }}
            >
              {t("Remove connection")}
            </button>
          </div>
        </details>
      </div>
    </article>
  );
}
