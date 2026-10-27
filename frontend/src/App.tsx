import { driveColorStyle } from "./driveColors";
import { DriveIcon, DriveAppearanceDialog } from "./DriveAppearance";
import { Preferences } from "./Preferences";
import { ThemeProvider, useTheme } from "./theme";
import { errorMessage as message, type LocalizedMessage } from "./messages";
import { DiagnosticReport } from "./DiagnosticReport";
import { DriveList } from "./DriveList";
import { DriveDiagnostics } from "./DriveDiagnostics";
import { DriveDetails, type DriveDetailSelection } from "./DriveDetails";
import { formatBytes as bytes } from "./formatBytes";
import { AccessBoundary, AccessSettings } from "./AccessProtection";
import { ConnectionEditor } from "./connection/ConnectionEditor";
import { useDialog } from "./useDialog";
import { DialogLayout } from "./DialogLayout";
import { LoadingStatus, Spinner } from "./Spinner";
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
  const [loadingSnapshot, setLoadingSnapshot] = useState(true);
  const [error, setError] = useState<string | LocalizedMessage>("");
  const [notice, setNotice] = useState("");
  const [editor, setEditor] = useState<{
    connection: Connection;
    auth: boolean;
    importDraft?: ImportDraft;
  }>();
  const [working, setWorking] = useState<Record<string, string>>({});
  const [driveDetail, setDriveDetail] = useState<DriveDetailSelection>();
  const [driveToolbar, setDriveToolbar] = useState<HTMLDivElement | null>(null);
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
    setLoadingSnapshot(true);
    try {
      const snapshot = await api.snapshot();
      if (!snapshotActive.current || request !== snapshotRequest.current) return;
      if (snapshot.access.locked) { onLocked(); return; }
      setData(snapshot);
      setTheme(snapshot.settings.theme || "system");
      setLanguage(isLanguage(snapshot.settings.language) ? snapshot.settings.language : "en");
    } catch (e) {
      if (snapshotActive.current && request === snapshotRequest.current) setError(message(e));
    } finally {
      if (snapshotActive.current && request === snapshotRequest.current) setLoadingSnapshot(false);
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
    propagate = false,
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
      if (propagate) throw e;
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
        {data && !data.settingsError && data.platform !== "linux" && !data.letters.length && (
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
        {!data && loadingSnapshot && <LoadingStatus large>{t("Preparing your drives…")}</LoadingStatus>}
        {Object.entries(working).filter(([key]) => key !== "restore" && !data?.connections.some((c) => c.id === key)).map(([key, operation]) =>
          <LoadingStatus key={key}>{t(operation)}</LoadingStatus>)}
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
        {data && !data.settingsError && !(data.mountAvailable ?? data.winFsp) && (
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
            <div className="summary-metrics">
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
            <div className="drive-toolbar-slot" ref={setDriveToolbar} />
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
          {data?.importDrafts?.map((draft) => <article className="drive" data-card-style={draft.definition.cardStyle || ""} style={driveColorStyle(draft.definition.cardStyle)} key={draft.id}>
            <DriveIcon connection={draft.definition} />
            <div className="drive-info"><h2>{draft.definition.name}</h2>
              <p className="server-path" title={draft.definition.mountFolder || `${draft.definition.letter}:`}>{data.providers.find((p) => p.id === draft.definition.provider)?.name} · {draft.definition.mountFolder || `${draft.definition.letter}:`}</p>
              <p role="status">{t("Authentication required for imported configuration")}</p>
              <p>{t("Supply application credentials and authenticate again. No credentials or pending files were imported.")}</p>
            </div>
            <div className="actions">
              <button disabled={!!data.settingsError || data.recoveryHold || !!data.engineError} onClick={() => setEditor({ connection: { ...blank(draft.definition.letter), ...draft.definition }, auth: true, importDraft: draft })}>{t("Authenticate")}</button>
              <button disabled={!!data.settingsError || data.recoveryHold || !!working[draft.id]} onClick={() => void run(draft.id, () => api.removeImportDraft(draft.id))}>{t("Remove draft")}</button>
            </div>
          </article>)}
          {data && <DriveList connections={data.connections.map(c => working[c.id] ? { ...c, busy: true } : c)} providers={data.providers} disabled={!!data.settingsError || data.recoveryHold}
            toolbarTarget={driveToolbar}
            onReorder={ids => run("drive-order", () => api.reorderConnections(ids), "", "Saving drive order…", true)}>{c => (
            <Drive
              key={c.id}
              connection={c}
              detailSelection={driveDetail}
              onDetailSelect={setDriveDetail}
              provider={data.providers.find((p) => p.id === c.provider)!}
              busy={!!working[c.id] || c.busy}
              operation={operationText[c.operation] || working[c.id] || ""}
              onAction={(action, operation, success) => void run(c.id, () => action(c.id), success, operation)}
              onDiagnostic={(work, operation) => run(c.id, work, "", operation, true)}
              onEdit={(auth) => setEditor({ connection: c, auth })}
              onAppearanceSaved={() => void refresh()}
            />
          )}</DriveList>}
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
        <footer className="drive-credits">{t("Open source · GPLv3")} · Go + rclone</footer>
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
  const { t, language } = useLanguage();
  const dialog = useDialog(onClose);
  const [info, setInfo] = useState<AboutInfo>();
  const [error, setError] = useState<string | LocalizedMessage>("");
  const [reportOpen, setReportOpen] = useState(false);
  useEffect(() => {
    let current = true;
    void api.about().then((value) => { if (current) setInfo(value); })
      .catch((e) => { if (current) setError(message(e)); });
    return () => { current = false; };
  }, []);
  return <DialogLayout dialogRef={dialog} titleID="about-title" title={t("About SDrive")} onClose={onClose} footer={<button onClick={onClose}>{t("Close")}</button>}>
    <p>{t("Open source · GPLv3")}</p>
    {error && <p role="alert" className="alert error">{t(error)}</p>}
    {!info && !error && <LoadingStatus>{t("Loading component versions…")}</LoadingStatus>}
    {info && <dl className="about-versions">{([
      ["SDrive", info.appVersion + (info.releaseDate && /^\d{4}-\d{2}-\d{2}$/.test(info.releaseDate) ? ` · ${new Intl.DateTimeFormat(language, { dateStyle: "medium", timeZone: "UTC" }).format(new Date(info.releaseDate))}` : "")], ["rclone (running)", info.rcloneVersion],
      [info.platform === "linux" ? "Linux" : "Windows", info.osVersion || info.windowsVersion], ["WinFsp (installed)", info.winFspVersion],
      ["WebView2 (installed)", info.webView2Version], ["Architecture", info.architecture],
    ]).filter(([label]) => info.platform !== "linux" || !["WinFsp (installed)", "WebView2 (installed)"].includes(label)).map(([label, value]) => <div key={label}><dt>{t(label)}</dt><dd>{value || t("Unavailable or not installed")}</dd></div>)}</dl>}
    <p>{t("Updates are installed manually. Use Exit safely and wait for uploads before running an installer.")}</p>
    <p>{t("Minimal diagnostics are kept locally with fixed limits. Nothing is sent automatically. Review the report before sharing.")}</p>
    <button onClick={() => setReportOpen(true)}>{t("View diagnostics")}</button>
    {reportOpen && <DiagnosticReport onClose={() => setReportOpen(false)} />}
  </DialogLayout>;
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
    <DialogLayout dialogRef={dialog} titleID="restore-title" title={t("Restore settings backup?")} onClose={onClose} closeDisabled={busy} centered
      footer={<><button disabled={busy} onClick={onClose}>{t("Cancel")}</button><button disabled={busy} className="primary" onClick={onConfirm}>{t("Confirm restoration")}</button></>}>
        {busy && <LoadingStatus>{t("Operation in progress…")}</LoadingStatus>}
        {error && <div className="alert error" role="alert">{t(error)}</div>}
        <p>
          {t("The backup may omit newer connections. Damaged settings will be archived; credentials and cache will be preserved. Automatic connection and cleanup will remain suspended until recovery is verified.")}
        </p>
    </DialogLayout>
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
  onDiagnostic,
  onAppearanceSaved,
}: {
  connection: ConnectionView;
  provider: Provider;
  detailSelection: DriveDetailSelection | undefined;
  onDetailSelect: (selection: DriveDetailSelection | undefined) => void;
  busy: boolean;
  operation: string;
  onAction: (action: (id: string) => Promise<void>, operation?: string, success?: string) => void;
  onEdit: (auth: boolean) => void;
  onDiagnostic: (work: () => Promise<void>, operation: string) => Promise<void>;
  onAppearanceSaved: () => void;
}) {
  const { t, language } = useLanguage();
  const { used, total } = c.capacity;
  const usage = used !== null && total !== null && Number.isFinite(used) && Number.isFinite(total) && total > 0 && used >= 0 && used <= total
    ? used / total * 100 : null;
  const usageText = usage === null ? "" : t("{percent} used · {used} of {total}", {
    percent: new Intl.NumberFormat(language, { style: "percent", maximumFractionDigits: 1 }).format(usage / 100),
    used: bytes(used), total: bytes(total),
  });
  const [diagnostics, setDiagnostics] = useState(false);
  const [appearance, setAppearance] = useState(false);
  const [confirmRemoval, setConfirmRemoval] = useState(false);
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
  const needsAttention = c.pending === null || c.pending > 0 || (c.recovery && !c.mounted) || (c.mounted && c.transportSecurity?.state !== "encrypted");
  const statusClass = busy ? "operating" : c.error || c.cache.error || c.uploads?.cacheErrors ? "failed" : needsAttention ? "attention" : c.mounted ? "connected" : "disconnected";
  return (
    <article className={`drive ${statusClass}`} data-card-style={c.cardStyle || ""} style={driveColorStyle(c.cardStyle)}>
      {appearance && <DriveAppearanceDialog connection={c} unavailable={busy} onClose={() => setAppearance(false)} onSave={onAppearanceSaved} />}
      {diagnostics && <DriveDiagnostics connection={c} busy={busy} onRun={onDiagnostic} onClose={() => setDiagnostics(false)} />}
      {confirmRemoval && <RemoveConnectionDialog name={c.name} unavailable={busy || c.mounted || c.recovery} onClose={() => setConfirmRemoval(false)} onConfirm={() => { setConfirmRemoval(false); onAction(api.remove); }} />}
      <DriveIcon connection={c} />
      <div className="drive-info">
        <div className="drive-heading"><h2>{c.name}</h2>
        <div role={busy ? "status" : undefined} className={`state status-tag ${busy ? "operating" : c.pending === null || c.recovery && !c.mounted || !!c.pending ? "attention" : c.mounted ? "connected" : "disconnected"}`}>
          {busy && <Spinner />}
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
        </div></div>
        <div className="drive-metadata"><span className={`type-tag type-${["smb", "sftp", "ftp"].includes(p.id) ? "network" : ["webdav", "nextcloud", "owncloud"].includes(p.id) ? "webdav" : ["s3", "b2", "azureblob", "gcs"].includes(p.id) ? "object" : "cloud"}`}>{t(p.name)}</span><span className="server-path" title={c.serverPath || c.path || t("Root")}>{c.serverPath || c.path || t("Root")}</span></div>
        {usage !== null && <div className="drive-capacity">
          <progress max={100} value={usage} aria-label={t("Space used on {name}", { name: c.name })} aria-valuetext={usageText} />
          <p>{usageText}</p>
        </div>}
        {p.experimental && <p className="drive-secondary">{t("Experimental")}</p>}
        {c.mountFolder && <p className="mount-folder"><strong>{t("Mounted at:")}</strong>{" "}<span title={c.mountFolder}>{c.mountFolder}</span></p>}
        {c.recovery && !c.mounted && (busy || c.pending === null) && <p className="inline-error">{t("Recovery required")}</p>}
        {!!c.error && <p className="inline-error">{t("Connection needs attention.")}</p>}
        {(!!c.cache.error || !!c.uploads?.cacheErrors) && <p className="inline-error">{t("Local cache needs attention.")}</p>}
        {c.mounted && c.transportSecurity?.state !== "encrypted" && <p className="inline-error">{t(c.transportSecurity?.state === "unencrypted" ? "Traffic unencrypted" : "Traffic encryption could not be verified")}</p>}
      </div>
      <div className="drive-controls">
        <div className="drive-tools">
          <DriveDetails connection={c} provider={p} selection={detailSelection} onSelect={onDetailSelect} />
        </div>
      <div className="actions">
        {c.mounted && <button disabled={busy} title={t("Open in Explorer")} onClick={() => onAction(api.open)}>{t("Open")}</button>}
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
        {!busy && !c.mounted && <button className="drive-detail-icon" data-tone="info" aria-label={t("Edit drive")}
          title={t("Edit drive")}
          onClick={() => { onDetailSelect(undefined); onEdit(!!c.recovery); }}>
          <svg viewBox="0 0 24 24" aria-hidden="true"><path d="m4 16-1 5 5-1L21 7l-4-4ZM14 6l4 4" /></svg>
        </button>}
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
            ...
          </summary>
          <div className="dropdown" onClickCapture={(event) => {
            if ((event.target as HTMLElement).closest("button:not(:disabled)")) closeMenu();
          }}>
            <button disabled={busy} onClick={() => { closeMenu(); setAppearance(true); }}>{t("Customize appearance")}</button>
            <button onClick={() => { closeMenu(); onDetailSelect(undefined); setDiagnostics(true); }}>{t("Drive diagnostics")}</button>
            <button
              disabled={busy || c.mounted || c.recovery}
              className="danger"
              onClick={() => setConfirmRemoval(true)}
            >
              {t("Remove connection")}
            </button>
          </div>
        </details>
      </div>
      </div>
    </article>
  );
}

function RemoveConnectionDialog({ name, unavailable, onClose, onConfirm }: { name: string; unavailable: boolean; onClose: () => void; onConfirm: () => void }) {
  const { t } = useLanguage();
  const dialog = useDialog(onClose);
  return <DialogLayout dialogRef={dialog} titleID="remove-title" title={t("Remove connection")} onClose={onClose} centered
    footer={<><button onClick={onClose}>{t("Cancel")}</button><button className="primary" disabled={unavailable} onClick={onConfirm}>{t("Remove connection")}</button></>}>
    <p>{t("Remove connection {name}? Remote files will be preserved.", { name })}</p>
  </DialogLayout>;
}
