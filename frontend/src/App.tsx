import { LanguageProvider, useLanguage, type Language } from "./i18n";
import { useEffect, useId, useRef, useState } from "react";
import {
  api,
  type Connection,
  type ConnectionView,
  type Provider,
  type Session,
  type Snapshot,
} from "./api";

const message = (error: unknown) =>
  error instanceof Error ? error.message : String(error);
const bytes = (value: number | null) => {
  if (value === null) return "—";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  const index = value > 0 ? Math.min(Math.floor(Math.log2(value) / 10), units.length - 1) : 0;
  return `${(value / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`;
};
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
  provider: "drive",
  path: "",
  letter,
  readOnly: false,
  autoMount: false,
  recovery: false,
  targetHash: "",
});

function useDialog(close: () => void) {
  const ref = useRef<HTMLElement>(null);
  const callback = useRef(close);
  callback.current = close;
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = ref.current;
    dialog?.querySelector<HTMLElement>("input,select,button")?.focus();
    const key = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        callback.current();
      }
      if (event.key === "Tab" && dialog) {
        const items = [
          ...dialog.querySelectorAll<HTMLElement>(
            'button:not(:disabled),input:not(:disabled),select:not(:disabled),summary,[tabindex="0"]',
          ),
        ].filter(
          (item) =>
            !item.closest("details:not([open])") || item.tagName === "SUMMARY",
        );
        const first = items[0],
          last = items.at(-1);
        if (!dialog.contains(document.activeElement)) {
          event.preventDefault();
          first?.focus();
        } else if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last?.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first?.focus();
        }
      }
    };
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("keydown", key);
      previous?.focus();
    };
  }, []);
  return ref;
}

export function App() {
  return (
    <LanguageProvider>
      <AppContent />
    </LanguageProvider>
  );
}

function AppContent() {
  const { t, setLanguage } = useLanguage();
  const [data, setData] = useState<Snapshot>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [editor, setEditor] = useState<{
    connection: Connection;
    auth: boolean;
  }>();
  const [working, setWorking] = useState<Record<string, string>>({});
  const [settings, setSettings] = useState(false);
  const [restoreConfirm, setRestoreConfirm] = useState(false);
  const snapshotRequest = useRef(0);
  const snapshotActive = useRef(true);
  const refresh = async () => {
    if (!snapshotActive.current) return;
    const request = ++snapshotRequest.current;
    try {
      const snapshot = await api.snapshot();
      if (!snapshotActive.current || request !== snapshotRequest.current) return;
      setData(snapshot);
      setLanguage(snapshot.settings.language === "pt-BR" ? "pt-BR" : "en");
    } catch (e) {
      if (snapshotActive.current && request === snapshotRequest.current) setError(message(e));
    }
  };
  useEffect(() => {
    snapshotActive.current = true;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      if (!active) return;
      await refresh();
      if (active) timer = setTimeout(poll, 3000);
    };
    void poll();
    return () => {
      active = false;
      snapshotActive.current = false;
      ++snapshotRequest.current;
      clearTimeout(timer);
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
        <footer>
          {t("Open source · GPLv3")}
          <br /> Go + rclone <br />
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
              !data || !!data.engineError || !!data.settingsError || data.recoveryHold || !data.letters.length
            }
            onClick={() =>
              setEditor({
                connection: blank(
                  data!.letters.includes("S") ? "S" : data!.letters[0],
                ),
                auth: true,
              })
            }
          >
            {t("+ New connection")}
          </button>
        </header>
        {data && !data.settingsError && !data.letters.length && (
          <p role="status" className="alert warning">{t("No drive letters are available. Free a letter between D and Z before creating a connection.")}</p>
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
        <section aria-label="Unidades" className="drives">
          {data?.connections.map((c) => (
            <Drive
              key={c.id}
              connection={c}
              provider={data.providers.find((p) => p.id === c.provider)!}
              busy={!!working[c.id] || c.busy}
              operation={operationText[c.operation] || working[c.id] || ""}
              onAction={(action, operation, success) => void run(c.id, () => action(c.id), success, operation)}
              onEdit={(auth) => setEditor({ connection: c, auth })}
            />
          ))}
        </section>
        {data && !data.settingsError && !data.recoveryHold && data.connections.length === 0 && (
          <section className="empty">
            <div className="empty-icon">▱</div>
            <h2>{t("One letter. All your files.")}</h2>
            <p>
              {t(
                "Connect an account or server and choose how it appears in Windows.",
              )}
            </p>
            <button
              disabled={!data.letters.length || !!data.engineError}
              onClick={() =>
                setEditor({
                  connection: blank(
                    data.letters.includes("S") ? "S" : data.letters[0],
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
    </div>
  );
}

function RestoreSettingsDialog({
  busy, error, onClose, onConfirm,
}: {
  busy: boolean;
  error: string;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const { t } = useLanguage();
  const dialog = useDialog(() => {
    if (!busy) onClose();
  });
  return (
    <div className="overlay">
      <section
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
      </section>
    </div>
  );
}

function Drive({
  connection: c,
  provider: p,
  busy,
  operation,
  onAction,
  onEdit,
}: {
  connection: ConnectionView;
  provider: Provider;
  busy: boolean;
  operation: string;
  onAction: (action: (id: string) => Promise<void>, operation?: string, success?: string) => void;
  onEdit: (auth: boolean) => void;
}) {
  const { t } = useLanguage();
  return (
    <article className="drive">
      <div className={`letter ${c.mounted ? "connected" : ""}`}>
        {c.letter}
        <span>:</span>
      </div>
      <div className="drive-info">
        <h2>{c.name}</h2>
        <p>
          {t(p.name)}
          {p.experimental ? " · Experimental" : ""} · {c.path || t("Root")}
          {c.readOnly ? t(" · Read only") : ""}
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
        {c.uploads && (
          <p>{t("{active} uploading · {queued} queued · {errors} cache errors", {
            active: c.uploads.active, queued: c.uploads.queued, errors: c.uploads.cacheErrors,
          })}</p>
        )}
        {c.recovery && !c.mounted && (
          <p>{t("Preserved destination: {provider} · {path}", { provider: t(p.name), path: c.path || t("Root") })}
            {c.pending !== null && <> · {t("{count} pending upload(s)", { count: c.pending })}</>}
            {" "}{t("Recover using the original account and destination, wait for uploads, then disconnect.")}
          </p>
        )}
        <p>{t("Approximate cache used: {used} · Pending file sizes: {pending}", {
          used: bytes(c.cache.usedBytes), pending: bytes(c.cache.pendingBytes),
        })}</p>
        <small>{t("Pending sizes are whole local files, not bytes remaining to upload.")}</small>
        <br />
        <small>{c.cache.checkedAt
          ? t("Local inspection: {time} (updated every 30 s)", { time: new Date(c.cache.checkedAt).toLocaleTimeString() })
          : t("Local cache inspection unavailable")}</small>
        {c.mounted && c.pending !== null && c.pending > 0 && <p>{t("Close open files and wait for uploads before disconnecting.")}</p>}
        {c.cache.error && <p className="inline-error" role="status">{t(c.cache.error)}</p>}
        {c.error && <p className="inline-error">{t(c.error)}</p>}
      </div>
      <div className="actions">
        <button
          disabled={busy}
          className={c.mounted ? "" : "primary"}
          onClick={() => onAction(c.mounted ? api.unmount : api.mount, c.mounted ? "Checking pending uploads…" : "Validating account…")}
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
        <details>
          <summary aria-label={t("More actions for {name}", { name: c.name })}>
            •••
          </summary>
          <div className="dropdown">
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

function SecretInput({ label, value, required, onChange }: {
  label: string;
  value: string;
  required: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useLanguage();
  const id = useId();
  const [revealed, setRevealed] = useState(false);
  return (
    <div className="secret-input">
      <label htmlFor={id}>{label}</label>
      <div className="secret-control">
        <input id={id} type={revealed ? "text" : "password"} autoComplete="off"
          required={required} value={value} onChange={(e) => onChange(e.target.value)} />
        <button type="button" aria-controls={id} aria-pressed={revealed}
          aria-label={t(revealed ? "Hide {field}" : "Show {field}", { field: label })}
          onClick={() => setRevealed((show) => !show)}>
          {t(revealed ? "Hide" : "Show")}
        </button>
      </div>
    </div>
  );
}

function ConnectionEditor({
  initial,
  auth,
  data,
  onRefresh,
  onClose,
}: {
  initial: Connection;
  auth: boolean;
  data: Snapshot;
  onRefresh: () => Promise<void>;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [connection, setConnection] = useState(initial);
  const [values, setValues] = useState<Record<string, string>>({});
  const [step, setStep] = useState(
    !auth ? "edit" : initial.id ? "access" : "service",
  );
  const [transport, setTransport] = useState("explicit");
  const [insecure, setInsecure] = useState(false);
  const [identity, setIdentity] = useState<{
    key: string;
    fingerprint: string;
  }>();
  const [trusted, setTrusted] = useState(false);
  const [sessionID, setSessionID] = useState("");
  const [session, setSession] = useState<Session>();
  const [answer, setAnswer] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(!!initial.id && auth);
  const [fieldsLoaded, setFieldsLoaded] = useState(!initial.id || !auth);
  const [error, setError] = useState("");
  const [folderError, setFolderError] = useState("");
  const [folders, setFolders] = useState<{ name: string; path: string }[]>([]);
  const [path, setPath] = useState(initial.path);
  const [directPath, setDirectPath] = useState(initial.path);
  const [savedID, setSavedID] = useState("");
  const [savedLetter, setSavedLetter] = useState("");
  const [operation, setOperation] = useState("");
  const p = data.providers.find((p) => p.id === connection.provider)!;
  const change = (patch: Partial<Connection>) =>
    setConnection((c) => ({ ...c, ...patch }));
  const action = async (work: () => Promise<void>) => {
    setBusy(true);
    setError("");
    try {
      await work();
    } catch (e) {
      setError(message(e));
    } finally {
      setOperation("");
      setBusy(false);
    }
  };
  useEffect(() => {
    let active = true;
    if (initial.id && auth)
      void api
        .fields(initial.id)
        .then((fields) => {
          if (active) {
            setValues(fields.values);
            if (fields.transport) setTransport(fields.transport);
            setFieldsLoaded(true);
          }
        })
        .catch((e) => {
          if (active) setError(message(e));
        })
        .finally(() => {
          if (active) setLoading(false);
        });
    return () => {
      active = false;
    };
  }, [initial.id, auth]);
  useEffect(() => {
    if (!sessionID) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const next = await api.status(sessionID);
        if (!active) return;
        setSession(next);
        if (next.authenticated && !next.complete)
          setStep((s) => (s === "auth" ? "folder" : s));
        if (!next.busy && next.question)
          setAnswer((previous) => previous || next.question?.DefaultStr || "");
      } catch (e) {
        if (active) setError(message(e));
      }
      if (active) timer = setTimeout(poll, 800);
    };
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [sessionID]);
  useEffect(() => {
    if (step !== "folder" || !sessionID || savedID) return;
    let active = true;
    setLoading(true);
    setFolderError("");
    setFolders([]);
    void api
      .folders(sessionID, path)
      .then((items) => {
        if (active) setFolders(items);
      })
      .catch((e) => {
        if (active) setFolderError(message(e));
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, [step, sessionID, path, savedID]);
  const close = () => {
    if (!busy)
      void action(async () => {
        if (sessionID) await api.end(sessionID);
        onClose();
      });
  };
  const dialog = useDialog(close);
  useEffect(() => {
    const heading = dialog.current?.querySelector<HTMLElement>("h3");
    if (heading) {
      heading.tabIndex = -1;
      heading.focus();
    } else
      dialog.current
        ?.querySelector<HTMLElement>("button,input,select")
        ?.focus();
  }, [step]);
  const unavailable =
    !initial.id &&
    p.available === false &&
    !(p.id === "gcs" && values.service_account_file);
  const noLetters = !initial.id && !savedID && !data.letters.length;
  const saved = data.connections.find((c) => c.id === savedID);
  const letterLocked = initial.recovery || !!saved?.recovery || !!saved?.mounted || !!saved?.busy;
  const letters = Array.from(new Set([
    ...((initial.id || savedID) ? [connection.letter] : []), ...data.letters,
  ])).sort();
  const question = session?.question;
  const setValue = (key: string, value: string) => {
    setValues((v) => ({ ...v, [key]: value }));
    if (["host", "port"].includes(key)) {
      setIdentity(undefined);
      setTrusted(false);
    }
  };
  const fields = (advanced: boolean) =>
    p.fields
      .filter(
        (f) =>
          !["client_id", "client_secret"].includes(f.key) &&
          !!f.advanced === advanced,
      )
      .map((f) => (
        <div className="credential-field" key={`${p.id}-${f.key}`}>
          {f.secret ? (
            <SecretInput label={t(f.label)}
              value={values[f.key] ?? f.default ?? ""} required={f.required}
              onChange={(value) => setValue(f.key, value)} />
          ) : <label>
          {t(f.label)}
          {f.choices ? (
            <select
              value={values[f.key] ?? f.default}
              onChange={(e) => setValue(f.key, e.target.value)}
            >
              {f.choices.map((value) => (
                <option key={value} value={value}>
                  {value === "api.pcloud.com"
                    ? t("United States")
                    : value === "eapi.pcloud.com"
                      ? t("Europe")
                      : value}
                </option>
              ))}
            </select>
          ) : (
            <input
              type="text"
              autoComplete="off"
              required={f.required}
              value={values[f.key] ?? f.default ?? ""}
              onChange={(e) => setValue(f.key, e.target.value)}
            />
          )}
          </label>}
          {f.hint && <small>{t(f.hint)}</small>}
          {f.file && (
            <button
              type="button"
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  const file = await api.selectFile(f.key);
                  if (file) setValue(f.key, file);
                })
              }
            >
              {t("Select file…")}
            </button>
          )}
        </div>
      ));
  const begin = () =>
    action(async () => {
      if (noLetters)
        throw new Error("No drive letters are available. Free a letter between D and Z before creating a connection.");
      if (!fieldsLoaded)
        throw new Error("Could not load access details. Cancel and try again.");
      const populated = { ...values };
      for (const f of p.fields)
        if (populated[f.key] === undefined && f.default)
          populated[f.key] = f.default;
      const id = await api.begin({
        connection: { ...connection, name: connection.name.trim() || p.name },
        values: populated,
        transport,
        allowInsecure: insecure,
        hostKey: trusted ? identity?.key || "" : "",
        guided: true,
      });
      setValues({});
      setSessionID(id);
      setStep("auth");
    });
  return (
    <div className="overlay">
      <section
        ref={dialog}
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="editor-title"
      >
        <div className="modal-heading">
          <div>
            <div className="eyebrow">
              {auth
                ? t("SERVICE → ACCESS → FOLDER → CONNECT")
                : t("CONFIGURE DRIVE")}
            </div>
            <h2 id="editor-title">
              {initial.id
                ? auth
                  ? t("Reconfigure account")
                  : t("Edit drive")
                : t("New connection")}
            </h2>
          </div>
          <button aria-label={t("Close setup")} disabled={busy} onClick={close}>
            {" "}
            ×{" "}
          </button>
        </div>
        {(error || session?.error) && (
          <div role="alert" className="alert error">
            {t(error || session?.error || "")}
          </div>
        )}
        {noLetters && <p role="alert">{t("No drive letters are available. Free a letter between D and Z before creating a connection.")}</p>}
        {savedID && step === "folder" && (
          <p role="status" className="alert warning">{t("Configuration saved. Retry uses this connection without creating another. You can also close setup and connect from My drives.")}</p>
        )}
        {initial.recovery && (
          <div className="alert warning">
            {t(
              "Only renew access to the same account. The original destination will be kept to recover your pending files.",
            )}
          </div>
        )}
        {step === "service" && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              setStep("access");
            }}
          >
            <label>
              {t("Connection type")}
              <select
                value={connection.provider}
                onChange={(e) => {
                  change({ provider: e.target.value, name: "" });
                  setValues({});
                  setIdentity(undefined);
                  setTrusted(false);
                  setTransport("explicit");
                  setInsecure(false);
                }}
              >
                {data.providers.map((item) => (
                  <option key={item.id} value={item.id}>
                    {t(item.name)}
                    {item.experimental ? " (experimental)" : ""}
                    {item.id === "gcs" && item.available === false
                      ? t(" — service account available")
                      : item.available === false
                      ? t(" — sign-in not configured")
                      : ""}
                  </option>
                ))}
              </select>
            </label>
            <label>
              {t("Drive name")}
              <input
                maxLength={80}
                value={connection.name}
                placeholder={t(p.name)}
                onChange={(e) => change({ name: e.target.value })}
              />
            </label>
            {unavailable && <p role="status">{t(p.id === "gcs" ? "Browser sign-in is unavailable. Continue to select a service account JSON file." : p.notice || "")}</p>}
            <div className="modal-actions">
              <button type="button" onClick={close}>
                {t("Cancel")}
              </button>
              <button className="primary" disabled={noLetters || (unavailable && p.id !== "gcs")}>{t("Continue")}</button>
            </div>
          </form>
        )}
        {step === "access" && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void begin();
            }}
          >
            <h3>
              {p.id === "gcs" && (p.available === false || values.service_account_file)
                ? t("Access to {provider}", { provider: p.name })
                : p.oauth
                ? t("Sign in to {provider}", { provider: p.name })
                : t("Access to {provider}", { provider: p.name })}
            </h3>
            <p>
              {p.id === "gcs" && (p.available === false || values.service_account_file)
                ? t("Select a service account JSON file authorized for your bucket. Browser sign-in is not required.")
                : p.oauth
                ? t(
                    "Sign in through your browser and authorize SDrive. Then return to this window.",
                  )
                : t(
                    "Enter your account credentials or the details provided by your storage administrator.",
                  )}
            </p>
            {unavailable && (
              <div className="alert warning" role="status">
                {t(p.id === "gcs" ? "Browser sign-in is unavailable. Continue to select a service account JSON file." : p.notice || "")}
              </div>
            )}
            {p.experimental && (
              <div className="alert warning">
                {t(
                  "This service is still experimental. Compatibility may vary by account.",
                )}
              </div>
            )}
            <fieldset disabled={loading || busy}>
              <legend>{t("Access details")}</legend>
              {fields(false)}
              {p.id === "ftp" && transport === "plain" && <p role="status">{t("This connection uses unencrypted FTP. Confirm permission in Advanced options before continuing.")}</p>}
              {p.id === "sftp" && (
                <div className="host-key">
                  <p>
                    {t(
                      "Verify the identity with your administrator before trusting this server.",
                    )}
                  </p>
                  <button
                    type="button"
                    onClick={() =>
                      void action(async () => {
                        setIdentity(
                          await api.probe(values.host || "", values.port || ""),
                        );
                        setTrusted(false);
                      })
                    }
                  >
                    {t("Verify SSH identity")}
                  </button>
                  {identity && (
                    <>
                      <code>{identity.fingerprint}</code>
                      <label className="check">
                        <input
                          type="checkbox"
                          checked={trusted}
                          onChange={(e) => setTrusted(e.target.checked)}
                        />
                        {t("I verified the fingerprint and trust this server")}
                      </label>
                    </>
                  )}
                </div>
              )}
              <details>
                <summary>{t("Advanced options")}</summary>
                {fields(true)}
                {p.id === "ftp" && (
                  <label>
                    {t("Access type")}
                    <select
                      value={transport}
                      onChange={(e) => setTransport(e.target.value)}
                    >
                      <option value="explicit">
                        {t("Secure FTPS (default)")}
                      </option>
                      <option value="implicit">{t("Implicit FTPS")}</option>
                      <option value="plain">{t("Unencrypted FTP")}</option>
                    </select>
                  </label>
                )}
                {["webdav", "nextcloud", "owncloud", "s3", "ftp"].includes(
                  p.id,
                ) && (
                  <label className="check">
                    <input
                      type="checkbox"
                      checked={insecure}
                      onChange={(e) => setInsecure(e.target.checked)}
                    />
                    {t("Allow unencrypted connection (HTTP / FTP)")}
                  </label>
                )}
                <label className="check">
                  <input
                    type="checkbox"
                    checked={connection.readOnly}
                    disabled={initial.recovery}
                    onChange={(e) => change({ readOnly: e.target.checked })}
                  />
                  {t("Mount read only")}
                </label>
                <label className="check">
                  <input
                    type="checkbox"
                    checked={connection.autoMount}
                    onChange={(e) => change({ autoMount: e.target.checked })}
                  />
                  {t("Connect automatically when SDrive starts")}
                </label>
              </details>
            </fieldset>
            <div className="modal-actions">
              <button
                type="button"
                disabled={busy}
                onClick={() => (initial.id ? close() : setStep("service"))}
              >
                {initial.id ? t("Cancel") : t("Back")}
              </button>
              <button
                className="primary"
                disabled={
                  busy ||
                  loading ||
                  !fieldsLoaded ||
                  noLetters ||
                  unavailable ||
                  (p.id === "sftp" && !trusted)
                }
              >
                {busy || loading
                  ? t("Preparing…")
                  : p.oauth && !values.service_account_file
                    ? t("Sign in with {provider}", { provider: p.name })
                    : t("Continue")}
              </button>
            </div>
          </form>
        )}
        {step === "auth" && (
          <div className="auth-flow">
            {!session || session.busy ? (
              <>
                <h3>{t("Waiting for authentication…")}</h3>
                <p>
                  {t(
                    "Complete sign-in in your browser if prompted, then return to SDrive.",
                  )}
                </p>
              </>
            ) : question ? (
              question.Unsupported ? (
                <p role="alert">{t(question.Help)}</p>
              ) : (
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    void action(async () => {
                      await api.answer(sessionID, answer);
                      setAnswer("");
                      setSession((s) => (s ? { ...s, busy: true } : s));
                    });
                  }}
                >
                  <h3>{t("Continue setup")}</h3>
                  <p>{t(question.Help)}</p>
                  {question.IsPassword || question.Sensitive ? (
                    <SecretInput key={`${sessionID}-${question.Name}-${session?.error || ""}`}
                      label={t(question.Name)} value={answer} required={question.Required}
                      onChange={setAnswer} />
                  ) : <label>
                    {t(question.Name)}
                    {question.Exclusive && question.Examples?.length ? (
                      <select
                        aria-label={t(question.Name)}
                        required={question.Required}
                        value={answer}
                        onChange={(e) => setAnswer(e.target.value)}
                      >
                        <option value="">{t("Select…")}</option>
                        {question.Examples.map((item) => (
                          <option key={item.Value} value={item.Value}>
                            {question.Name === "Account type" ||
                            question.Type === "bool"
                              ? t(item.Help || item.Value)
                              : item.Help || item.Value}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <input
                        aria-label={t(question.Name)}
                        autoComplete="off"
                        type="text"
                        required={question.Required}
                        value={answer}
                        onChange={(e) => setAnswer(e.target.value)}
                      />
                    )}
                  </label>}
                  <button disabled={busy} className="primary">
                    {t("Continue")}
                  </button>
                </form>
              )
            ) : (
              <p>{t("Access was not completed. Cancel and try again.")}</p>
            )}
            <button disabled={busy} onClick={close}>
              {t("Cancel setup")}
            </button>
          </div>
        )}
        {step === "folder" && (
          <div>
            <h3>{t("Choose what to connect")}</h3>
            <p>{t("Select a folder or keep the entire storage.")}</p>
            <p aria-live="polite">
              <strong>{t("Current folder:")}</strong>{" "}
              {path || t("Entire storage")}
            </p>
            {folderError && (
              <p role="alert">
                {t(folderError)}{" "}
                {t(
                  "If your account only allows access to a specific destination, enter its address below.",
                )}
              </p>
            )}
            {loading ? (
              <p role="status">{t("Loading folders…")}</p>
            ) : (
              <div className="folder-list" aria-label={t("Available folders")}>
                {path && !initial.recovery && (
                  <button
                    disabled={!!savedID || busy}
                    onClick={() =>
                      setPath(path.split("/").slice(0, -1).join("/"))
                    }
                  >
                    {t("↑ Parent folder")}
                  </button>
                )}
                {!initial.recovery &&
                  folders.map((item) => (
                    <button
                      key={item.path}
                      disabled={!!savedID || busy}
                      onClick={() => setPath(item.path)}
                    >
                      ▱ {item.name}
                    </button>
                  ))}
                {!folderError && folders.length === 0 && (
                  <p>{t("No subfolders here.")}</p>
                )}
              </div>
            )}
            {!initial.recovery && (
              <details>
                <summary>{t("Enter destination directly")}</summary>
                <label>
                  {t("Folder, bucket or share")}
                  <input
                    value={directPath}
                    disabled={!!savedID || busy}
                    placeholder={t("Example: documents or my-bucket/folder")}
                    onChange={(e) => setDirectPath(e.target.value)}
                  />
                  <small>
                    {t(
                      "Use the path provided by your administrator, separated by /.",
                    )}
                  </small>
                </label>
                <button
                  disabled={!!savedID || busy}
                  onClick={() => setPath(directPath)}
                >
                  {t("Open destination")}
                </button>
              </details>
            )}
            <label>
              {t("Windows drive letter")}
              <select aria-label={t("Windows drive letter")} value={letters.includes(connection.letter) ? connection.letter : ""} disabled={busy || letterLocked || noLetters}
                onChange={(e) => change({ letter: e.target.value })}>
                {!letters.includes(connection.letter) && <option value="" disabled>{t("Select…")}</option>}
                {letters.map((letter) => <option key={letter} value={letter}>{letter}:</option>)}
              </select>
              <small>
                {t(letterLocked
                  ? "Keep the saved letter while recovery or an operation is pending. Resolve pending files before changing it."
                  : "Choose a free letter. If connecting fails, you can correct it here and retry the saved connection.")}
              </small>
            </label>
            <div className="modal-actions">
              <button disabled={busy} onClick={close}>
                {savedID ? t("Close") : t("Cancel")}
              </button>
              <button
                className="primary"
                disabled={busy || loading || !data.winFsp || noLetters || (!savedID && !initial.id && !data.letters.includes(connection.letter))}
                onClick={() =>
                  void action(async () => {
                    try {
                      let id = savedID;
                      if (!id || savedLetter !== connection.letter) {
                        setOperation("Validating destination…");
                        id = await api.confirm(sessionID, path, connection.letter);
                        setSavedID(id);
                        setSavedLetter(connection.letter);
                      }
                      setOperation("Mounting drive…");
                      await api.mount(id);
                      setStep("complete");
                    } finally {
                      await onRefresh();
                    }
                  })
                }
              >
                {busy ? t(operation || "Validating destination…") : savedID ? t("Retry connection") : t("Connect drive")}
              </button>
            </div>
            {!data.winFsp && (
              <p role="alert">
                {t("Run the SDrive installer again to prepare drive mounting.")}
              </p>
            )}
          </div>
        )}
        {step === "complete" && (
          <div className="auth-flow">
            <div className="complete-icon">✓</div>
            <h3>{t("Drive connected")}</h3>
            <p>
              {t("Your files are available on Windows drive {letter}:.", {
                letter: connection.letter,
              })}
            </p>
            <div className="modal-actions">
              <button onClick={close}>{t("Done")}</button>
              <button
                className="primary"
                disabled={busy}
                onClick={() => void action(() => api.open(savedID))}
              >
                {t("Open in Explorer")}
              </button>
            </div>
          </div>
        )}
        {step === "edit" && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void action(async () => {
                await api.update(connection);
                onClose();
              });
            }}
          >
            <label>
              {t("Drive name")}
              <input
                required
                maxLength={80}
                value={connection.name}
                onChange={(e) => change({ name: e.target.value })}
              />
            </label>
            <label>
              {t("Windows drive letter")}
              <select
                value={connection.letter}
                onChange={(e) => change({ letter: e.target.value })}
              >
                {Array.from(new Set([connection.letter, ...data.letters]))
                  .sort()
                  .map((letter) => (
                    <option key={letter} value={letter}>
                      {letter}:
                    </option>
                  ))}
              </select>
            </label>
            <label>
              {t("Folder / bucket / share")}
              <input
                value={connection.path}
                onChange={(e) => change({ path: e.target.value })}
              />
            </label>
            <label className="check">
              <input
                type="checkbox"
                checked={connection.readOnly}
                disabled={connection.provider === "drive"}
                onChange={(e) => change({ readOnly: e.target.checked })}
              />
              {t("Mount read only")}
            </label>
            {connection.provider === "drive" && <p>{t("Use Reconfigure account to change Google Drive access mode and update OAuth consent.")}</p>}
            <label className="check">
              <input
                type="checkbox"
                checked={connection.autoMount}
                onChange={(e) => change({ autoMount: e.target.checked })}
              />
              {t("Connect automatically when SDrive starts")}
            </label>
            <div className="modal-actions">
              <button type="button" disabled={busy} onClick={close}>
                {t("Cancel")}
              </button>
              <button className="primary" disabled={busy}>
                {t("Save changes")}
              </button>
            </div>
          </form>
        )}
      </section>
    </div>
  );
}
function Preferences({
  data,
  onClose,
}: {
  data: Snapshot;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const [settings, setSettings] = useState(data.settings);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const dialog = useDialog(() => {
    if (!busy) onClose();
  });
  return (
    <div className="overlay">
      <section
        ref={dialog}
        className="modal small"
        role="dialog"
        aria-modal="true"
        aria-labelledby="preferences-title"
      >
        <h2 id="preferences-title">{t("Preferences")}</h2>
        {error && (
          <div role="alert" className="alert error">
            {t(error)}
          </div>
        )}
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setBusy(true);
            try {
              await api.settings(settings);
              onClose();
            } catch (e) {
              setError(message(e));
            } finally {
              setBusy(false);
            }
          }}
        >
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
              <option value="en">English</option>
              <option value="pt-BR">Português (Brasil)</option>
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
          <label>
            {t("Cache target per connection (GiB)")}
            <input
              type="number"
              min="1"
              max="1024"
              value={settings.cacheGiB}
              onChange={(e) =>
                setSettings((s) => ({ ...s, cacheGiB: Number(e.target.value) }))
              }
            />
          </label>
          <p>
            {t(
              "Applied on the next mount. Open files and pending uploads may exceed the target. SDrive reserves 5 GiB of free space before connecting.",
            )}
          </p>
          <p>
            {t(
              "Credentials are encrypted. The cache contains local files accessible to your Windows user.",
            )}
          </p>
          <div className="modal-actions">
            <button type="button" disabled={busy} onClick={onClose}>
              {t("Cancel")}
            </button>
            <button className="primary" disabled={busy}>
              {t("Save")}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
