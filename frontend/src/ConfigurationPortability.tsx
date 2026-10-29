import { themeLabels } from "./theme";
import { useState } from "react";
import { api, type ImportPreview, type Snapshot } from "./api";
import { useDialog } from "./useDialog";
import { DialogLayout } from "./DialogLayout";
import { useLanguage } from "./i18n";
import { errorMessage } from "./messages";
import { LoadingStatus } from "./Spinner";

export function ConfigurationPortability({ data, onRefresh }: { data: Snapshot; onRefresh: () => Promise<void> }) {
  const { t } = useLanguage();
  const [preview, setPreview] = useState<ImportPreview>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const run = async (work: () => Promise<void>) => {
    setBusy(true); setError("");
    try { await work(); } catch (e) { setError(t(errorMessage(e))); } finally { setBusy(false); }
  };
  return <section aria-label={t("Portable configuration")}>
    <h3>{t("Portable configuration")}</h3>
    {busy && <LoadingStatus>{t("Operation in progress…")}</LoadingStatus>}
    <p>{t("Exports contain names, servers and paths, but no credentials. Keep the file private. Importing does not restore a local backup or connect drives.")}</p>
    <button disabled={busy || !!data.settingsError} onClick={() => void run(api.exportConfiguration)}>{t("Export configuration")}</button>{" "}
    <button disabled={busy || !!data.settingsError || data.recoveryHold} onClick={() => void run(async () => { const next = await api.previewImport(); if (next) setPreview(next); })}>{t("Import configuration")}</button>
    {error && <p role="alert">{error}</p>}
    {preview && <ImportConfigurationDialog preview={preview} data={data} onClose={() => setPreview(undefined)} onSaved={async () => { setPreview(undefined); await onRefresh(); }} />}
  </section>;
}

export function ImportConfigurationDialog({ preview, data, onClose, onSaved }: { preview: ImportPreview; data: Snapshot; onClose: () => void; onSaved: () => Promise<void> }) {
  const { t } = useLanguage();
  const [document, setDocument] = useState(() => data.platform === "linux" ? { ...preview.document, connections: preview.document.connections.map((entry) => ({ ...entry, letter: "", mountMode: "local", mountFolder: (entry.mountFolder || "").startsWith("/") ? entry.mountFolder : "" })) } : preview.document);
  const [selected, setSelected] = useState(document.connections.map(() => true));
  const [preferences, setPreferences] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const dialog = useDialog(() => { if (!busy) onClose(); });
  const definitions = document.connections.filter((_, i) => selected[i]);
  const existing = [...data.connections, ...(data.importDrafts || []).map((d) => d.definition)];
  const linux = data.platform === "linux";
  const pathKey = (value: string) => linux ? value.replace(/\/+$/, "") : normalize(value);
  const normalize = (value: string) => value.trim().replaceAll("/", "\\").replace(/\\+$/, "").toLowerCase();
  const invalid = document.connections.map((entry, index) => {
    if (!selected[index]) return "";
    const others = [...existing, ...document.connections.filter((_, i) => i !== index && selected[i])];
    if (!entry.name.trim() || [...entry.name.trim()].length > 80 || /[\0\r\n]/.test(entry.name)) return "Enter a name with 1 to 80 characters.";
    if (others.some((other) => normalize(other.name) === normalize(entry.name))) return "Resolve duplicate names before importing.";
    if (linux ? !(entry.mountFolder || "").startsWith("/") || entry.mountFolder === "/" : entry.mountFolder ? !/^[a-z]:\\/i.test(entry.mountFolder) : !data.letters.includes(entry.letter)) return "Choose an available mount destination.";
    if (others.some((other) => entry.mountFolder && other.mountFolder
      ? pathKey(entry.mountFolder) === pathKey(other.mountFolder) || pathKey(entry.mountFolder).startsWith(pathKey(other.mountFolder)+(linux ? "/" : "\\")) || pathKey(other.mountFolder).startsWith(pathKey(entry.mountFolder)+(linux ? "/" : "\\"))
      : !entry.mountFolder && !other.mountFolder && entry.letter.toUpperCase() === other.letter.toUpperCase())) return "Resolve conflicting mount destinations before importing.";
    return "";
  });
  const save = async () => {
    setBusy(true); setError("");
    try { await api.commitImport(preview.id, { ...document, connections: definitions }, preferences); await onSaved(); }
    catch (e) { setError(t(errorMessage(e))); } finally { setBusy(false); }
  };
  const patch = (index: number, values: Partial<typeof document.connections[number]>) => setDocument((d) => ({ ...d, connections: d.connections.map((c, i) => i === index ? { ...c, ...values } : c) }));
  return <DialogLayout dialogRef={dialog} titleID="import-title" title={t("Review imported configuration")} onClose={onClose} closeDisabled={busy}
    footer={<><button disabled={busy} onClick={onClose}>{t("Cancel")}</button><button className="primary" disabled={busy || invalid.some(Boolean) || (!definitions.length && !preferences)} onClick={() => void save()}>{t("Save imported drafts")}</button></>}>
    {busy && <LoadingStatus>{t("Operation in progress…")}</LoadingStatus>}
    <p>{t("Selected entries are saved as drafts. Supply application credentials and authenticate each draft before connecting. Existing recovery data stays in its original profile.")}</p>
    {document.connections.map((entry, index) => <fieldset key={index} disabled={busy}>
      <legend><label><input type="checkbox" checked={selected[index]} onChange={(e) => setSelected((s) => s.map((value, i) => i === index ? e.target.checked : value))} />{entry.name || t("Imported connection")}</label></legend>
      <p>{data.providers.find((p) => p.id === entry.provider)?.name} · {entry.path || t("Root")}</p>
      <label>{t("Drive name")}<input value={entry.name} maxLength={80} aria-invalid={!!invalid[index]} aria-describedby={invalid[index] ? `import-error-${index}` : undefined} onChange={(e) => patch(index, { name: e.target.value })} /></label>
      {!linux && <label>{t("Mount into")}<select value={entry.mountFolder ? "folder" : "letter"} onChange={(e) => patch(index, e.target.value === "folder" ? { mountFolder: "C:\\", mountMode: "local", letter: "" } : { mountFolder: "", letter: data.letters[0] || "S" })}><option value="letter">{t("Drive letter")}</option><option value="folder">{t("Local folder")}</option></select></label>}
      {linux || entry.mountFolder ? <label>{t("Local mount folder")}<input value={entry.mountFolder} onChange={(e) => patch(index, { mountFolder: e.target.value })} /></label> : <>
        <label>{t("Windows drive letter")}<select value={entry.letter} onChange={(e) => patch(index, { letter: e.target.value })}>{Array.from(new Set([entry.letter, ...data.letters])).filter(Boolean).map((letter) => <option key={letter}>{letter}</option>)}</select></label>
        <label>{t("Drive presentation")}<select value={entry.mountMode || "network"} onChange={(e) => patch(index, { mountMode: e.target.value })}><option value="network">{t("Network drive")}</option><option value="local">{t("Local disk")}</option></select></label>
      </>}
      {invalid[index] && <p id={`import-error-${index}`} role="alert">{t(invalid[index])}</p>}
    </fieldset>)}
    <label><input type="checkbox" disabled={busy} checked={preferences} onChange={(e) => setPreferences(e.target.checked)} />{t("Apply imported theme, language, cache timing and desktop preferences")}</label>
    <p>{t("Theme")}: {t(themeLabels[document.preferences.theme] || "System")}</p>
    <p>{t("Imported preferences: {language}, {cache} GiB, retention {hours} h, upload delay {seconds} s.", { language: document.preferences.language, cache: document.preferences.cacheGiB, hours: document.preferences.cacheMaxAgeHours ?? 24, seconds: document.preferences.writeBackSeconds ?? 5 })}</p>
    <p>{t("Show service icons for drive letters")}: {t(document.preferences.showServiceIcons ? "Enabled" : "Disabled")}</p>
    <p>{t("Appearance preferences are included when applying imported preferences.")}</p>
    <p>{t("Windows startup, proxy credentials and cache location stay local.")}</p>
    {error && <p role="alert">{error}</p>}
  </DialogLayout>;
}
