import { useId, type ReactNode } from "react";
import { DialogLayout } from "./DialogLayout";
import { useDialog } from "./useDialog";
import type { ConnectionView, Provider } from "./api";
import { useLanguage } from "./i18n";
import { TransportStatus } from "./TransportSecurity";
import { formatBytes as bytes } from "./formatBytes";

export interface DriveDetailSelection { connectionID: string; topic: string }

type DetailTopic = "destination" | "options" | "uploads" | "cache" | "capacity" | "security" | "alerts";

export function DriveDetails({ connection: c, provider: p, selection, onSelect }: {
  connection: ConnectionView;
  provider: Provider;
  selection: DriveDetailSelection | undefined;
  onSelect: (selection: DriveDetailSelection | undefined) => void;
}) {
  const { t } = useLanguage();
  const id = useId();
  const topic = selection?.topic || "destination";
  const dialog = useDialog(() => onSelect(undefined));
  const needsRecovery = c.recovery && !c.mounted;
  const hasAlerts = !!(c.error || c.cache.error || needsRecovery || c.pending === null || c.uploads?.cacheErrors ||
    (c.mounted && c.transportSecurity?.state !== "encrypted"));
  const items: { key: DetailTopic; title: string; attention?: boolean; content: ReactNode }[] = [
    { key: "destination", title: "Remote destination", content: <p>{t(p.name)} · {c.path || t("Root")}</p> },
    { key: "options", title: "Connection options", content: <ul aria-label={t("Connection options")}>
      {[
        c.readOnly ? t("Read only") : t("Read and write"),
        t(c.autoMount ? "Automatic mounting enabled" : "Automatic mounting disabled"),
        t(c.sessionOnly ? "Sign in on every connection" : "Saved credentials"),
        t(c.mountFolder ? "Local folder" : c.mountMode === "local" ? "Local disk" : "Network drive"),
        t(c.useProxy ? "Proxy requested" : "Direct connection"),
        c.uploadLimit ? t("Upload limit: {size}/s", { size: bytes(c.uploadLimit) }) : t("Upload limit: unlimited"),
        c.downloadLimit ? t("Download limit: {size}/s", { size: bytes(c.downloadLimit) }) : t("Download limit: unlimited"),
      ].map((label) => <li key={label}>{label}</li>)}
    </ul> },
    { key: "uploads", title: "Upload details", attention: c.pending === null || !!c.uploads?.cacheErrors, content: <>
      <p>{c.pending === null ? t("Pending uploads unknown") : t("{count} pending upload(s)", { count: c.pending })}</p>
      <p>{c.uploads ? t("{active} uploading · {queued} queued · {errors} cache errors", {
        active: c.uploads.active, queued: c.uploads.queued, errors: c.uploads.cacheErrors,
      }) : t("Upload activity is currently unknown.")}</p>
      {c.mounted && c.pending !== null && c.pending > 0 && <p>{t("Close open files and wait for uploads before disconnecting.")}</p>}
    </> },
    { key: "cache", title: "Local cache", attention: !!c.cache.error, content: <>
      <p>{t("Approximate cache used: {used} · Pending file sizes: {pending}", { used: bytes(c.cache.usedBytes), pending: bytes(c.cache.pendingBytes) })}</p>
      <p>{t("Pending sizes are whole local files, not bytes remaining to upload.")}</p>
      <p>{c.cache.checkedAt ? t("Local inspection: {time} (updated every 30 s)", { time: new Date(c.cache.checkedAt).toLocaleTimeString() }) : t("Local cache inspection unavailable")}</p>
      {c.cache.error && <p>{t(c.cache.error)}</p>}
    </> },
    { key: "capacity", title: "Remote capacity", content: <>
      <p>{t("Remote capacity: {total} · Used: {used} · Available: {free}", { total: bytes(c.capacity.total), used: bytes(c.capacity.used), free: bytes(c.capacity.free) })}</p>
      <p>{t(c.capacity.state === "unsupported" ? "This service does not report capacity." : c.capacity.state === "available" ? "Capacity is reported by the provider and may cover the whole account. It is separate from the local cache disk." : "Remote capacity is currently unknown.")}</p>
    </> },
    { key: "security", title: "Connection security", attention: c.mounted && c.transportSecurity?.state !== "encrypted", content:
      c.mounted ? <TransportStatus connection={c} /> : <p>{t("Transport security is unavailable while disconnected.")}</p> },
    { key: "alerts", title: "Alerts and recovery", attention: hasAlerts, content: <>
      {!hasAlerts && <p>{t("No alerts or recovery required.")}</p>}
      {c.pending === null && <p>{t("Pending uploads unknown")}</p>}
      {!!c.uploads?.cacheErrors && <p>{t("{active} uploading · {queued} queued · {errors} cache errors", { active: c.uploads.active, queued: c.uploads.queued, errors: c.uploads.cacheErrors })}</p>}
      {needsRecovery && <p>{t("Preserved destination: {provider} · {path}", { provider: t(p.name), path: c.path || t("Root") })}
        {c.pending !== null && <> · {t("{count} pending upload(s)", { count: c.pending })}</>}{" "}
        {t("Recover using the original account and destination, wait for uploads, then disconnect.")}</p>}
      {c.cache.error && <p>{t(c.cache.error)}</p>}
      {c.error && <p>{t(c.errorMessage || c.error)}</p>}
      {c.mounted && c.transportSecurity?.state !== "encrypted" && <TransportStatus connection={c} />}
    </> },
  ];
  const item = items.find((entry) => entry.key === topic) || items[0];
  return <DialogLayout dialogRef={dialog} titleID={id} title={t("Drive details") + " · " + c.name} onClose={() => onSelect(undefined)}
    footer={<button onClick={() => onSelect(undefined)}>{t("Close")}</button>}>
    <div className="edit-tabs detail-tabs" role="tablist" aria-label={t("Drive details")}>
      {items.map((entry, index) => <button key={entry.key} type="button" role="tab" id={id + "-" + entry.key}
        aria-selected={item.key === entry.key} aria-controls={id + "-content"} tabIndex={item.key === entry.key ? 0 : -1}
        className={entry.attention ? "attention" : undefined}
        onClick={() => onSelect({ connectionID: c.id, topic: entry.key })}
        onKeyDown={(event) => {
          if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
          event.preventDefault();
          const next = event.key === "Home" ? items[0] : event.key === "End" ? items[items.length - 1]
            : items[(index + (event.key === "ArrowRight" ? 1 : items.length - 1)) % items.length];
          onSelect({ connectionID: c.id, topic: next.key });
          document.getElementById(id + "-" + next.key)?.focus();
        }}>{t(entry.title)}{entry.attention && <span aria-hidden="true"> !</span>}</button>)}
    </div>
    <section id={id + "-content"} role="tabpanel" aria-labelledby={id + "-" + item.key} tabIndex={0}>
      <h3>{t(item.title)}</h3>{item.content}
    </section>
  </DialogLayout>;
}
