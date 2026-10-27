import { useEffect, useId, useLayoutEffect, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import type { ConnectionView, Provider } from "./api";
import { useLanguage } from "./i18n";
import { TransportStatus } from "./TransportSecurity";
import { formatBytes as bytes } from "./formatBytes";
import { NetworkDetails } from "./NetworkDetails";

export interface DriveDetailSelection { connectionID: string; topic: string }

const icons = {
  destination: "M3 7V5h6l2 2h10v12H3Z",
  options: "M4 6h16M4 12h16M4 18h16M8 3v6M16 9v6M10 15v6",
  uploads: "M12 16V3m-5 5 5-5 5 5M4 16v5h16v-5",
  cache: "M5 3h9l5 5v13H5ZM14 3v6h5M8 13h8M8 17h5",
  capacity: "M5 4h14l3 13H2ZM2 17v4h20v-4M16 19h3",
  network: "M7 3v14m-4-4 4 4 4-4M17 21V7m-4 4 4-4 4 4",
  security: "M12 2 3 6v6c0 5 9 10 9 10s9-5 9-10V6Z",
  alerts: "M12 3 2 21h20ZM12 9v5M12 17v1",
};

export function DriveDetails({ connection: c, provider: p, selection, onSelect }: {
  connection: ConnectionView;
  provider: Provider;
  selection: DriveDetailSelection | undefined;
  onSelect: (selection: DriveDetailSelection | undefined) => void;
}) {
  const { t } = useLanguage();
  const id = useId();
  const strip = useRef<HTMLDivElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const hovered = useRef(false);
  const focusedTopic = useRef<string | undefined>(undefined);
  const leaveTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const topic = selection?.connectionID === c.id ? selection.topic : undefined;
  const needsRecovery = c.recovery && !c.mounted;
  const hasAlerts = !!(c.error || c.cache.error || needsRecovery || c.pending === null || c.uploads?.cacheErrors ||
    (c.mounted && c.transportSecurity?.state !== "encrypted"));
  const uploadErrors = !!c.uploads?.cacheErrors;
  const transfersVisible = c.pending === null || c.pending > 0 || !!c.uploads?.active || !!c.uploads?.queued || uploadErrors;
  const cacheVisible = c.mounted || needsRecovery || c.pending === null || c.pending > 0 || !!c.cache.usedBytes || !!c.cache.pendingBytes || !!c.cache.error;
  const cacheUnknown = !c.cache.checkedAt || c.cache.usedBytes === null || c.cache.pendingBytes === null || c.pending === null;
  const security = c.transportSecurity?.state;
  const alertError = !!(c.error || c.cache.error || uploadErrors || needsRecovery || c.mounted && security === "unencrypted");
  const items: { key: keyof typeof icons; title: string; visible?: boolean; tone?: "info" | "success" | "warning" | "danger" | "inactive"; content: ReactNode }[] = [
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
    { key: "uploads", title: "Upload details", visible: transfersVisible, tone: uploadErrors ? "danger" : c.pending === null ? "warning" : c.uploads?.active ? "info" : "warning", content: <>
      <p>{c.pending === null ? t("Pending uploads unknown") : t("{count} pending upload(s)", { count: c.pending })}</p>
      <p>{c.uploads ? t("{active} uploading · {queued} queued · {errors} cache errors", {
        active: c.uploads.active, queued: c.uploads.queued, errors: c.uploads.cacheErrors,
      }) : t("Upload activity is currently unknown.")}</p>
      {c.mounted && c.pending !== null && c.pending > 0 && <p>{t("Close open files and wait for uploads before disconnecting.")}</p>}
    </> },
    { key: "cache", title: "Local cache", visible: cacheVisible, tone: c.cache.error ? "danger" : cacheUnknown ? "warning" : "info", content: <>
      <p>{t("Approximate cache used: {used} · Pending file sizes: {pending}", { used: bytes(c.cache.usedBytes), pending: bytes(c.cache.pendingBytes) })}</p>
      <p>{t("Pending sizes are whole local files, not bytes remaining to upload.")}</p>
      <p>{c.cache.checkedAt ? t("Local inspection: {time} (updated every 30 s)", { time: new Date(c.cache.checkedAt).toLocaleTimeString() }) : t("Local cache inspection unavailable")}</p>
      {c.cache.error && <p>{t(c.cache.error)}</p>}
    </> },
    { key: "network", title: "Network", visible: c.mounted || !!c.network?.state, tone: !c.network?.checkedAt || c.network?.state === "stale" || c.network?.partial ? "warning" : "info", content:
      <NetworkDetails network={c.network || { state: "unavailable", partial: false, startedAt: null, checkedAt: null, sent: null, received: null, filesSent: null, filesReceived: null }} mounted={c.mounted} /> },
    { key: "capacity", title: "Remote capacity", visible: c.mounted && c.capacity.state !== "unsupported", tone: c.capacity.state === "available" ? "info" : "warning", content: <>
      <p>{t("Remote capacity: {total} · Used: {used} · Available: {free}", { total: bytes(c.capacity.total), used: bytes(c.capacity.used), free: bytes(c.capacity.free) })}</p>
      <p>{t(c.capacity.state === "available" ? "Capacity is reported by the provider and may cover the whole account. It is separate from the local cache disk." : "Remote capacity is currently unknown.")}</p>
    </> },
    { key: "security", title: "Connection security", visible: c.mounted, tone: security === "encrypted" ? "success" : security === "unencrypted" ? "inactive" : "warning", content:
      <TransportStatus connection={c} /> },
    { key: "alerts", title: "Alerts and recovery", visible: hasAlerts, tone: alertError ? "danger" : "warning", content: <>
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
  const visibleItems = items.filter((entry) => entry.visible !== false);
  const item = visibleItems.find((entry) => entry.key === topic);
  useLayoutEffect(() => {
    if (topic && !item) onSelect(undefined);
    if (focusedTopic.current && !visibleItems.some((entry) => entry.key === focusedTopic.current)) {
      focusedTopic.current = undefined;
      if (document.activeElement === document.body) strip.current?.querySelector("button")?.focus();
    }
  });
  const cancelLeave = () => clearTimeout(leaveTimer.current);
  const enter = () => { hovered.current = true; cancelLeave(); };
  const leave = () => {
    hovered.current = false;
    cancelLeave();
    if (!topic) return;
    leaveTimer.current = setTimeout(() => {
      if (!strip.current?.contains(document.activeElement)) onSelect(undefined);
    }, 120);
  };
  useEffect(() => () => clearTimeout(leaveTimer.current), []);
  useEffect(() => {
    if (!topic) return;
    const outside = (event: Event) => {
      const target = event.target as Node;
      if (!strip.current?.contains(target) && !panel.current?.contains(target)) onSelect(undefined);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") { cancelLeave(); onSelect(undefined); }
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("focusin", outside);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", outside);
      document.removeEventListener("focusin", outside);
      document.removeEventListener("keydown", escape);
      cancelLeave();
    };
  }, [topic, onSelect]);
  useLayoutEffect(() => {
    if (!topic) return;
    const position = () => {
      const trigger = strip.current?.querySelector(`[data-topic="${topic}"]`);
      const tooltip = panel.current;
      if (!trigger || !tooltip) return;
      const rect = trigger.getBoundingClientRect();
      const scrollTop = tooltip.scrollTop;
      tooltip.style.maxHeight = `${window.innerHeight - 16}px`;
      const { width, height } = tooltip.getBoundingClientRect();
      tooltip.style.left = `${Math.max(8, Math.min(rect.left, window.innerWidth - width - 8))}px`;
      const below = window.innerHeight - rect.bottom - 16;
      const above = rect.top - 16;
      const opensBelow = height <= below || below >= above;
      tooltip.style.maxHeight = `${Math.max(40, Math.min(window.innerHeight - 16, opensBelow ? below : above))}px`;
      const actualHeight = tooltip.getBoundingClientRect().height;
      const top = opensBelow ? rect.bottom + 8 : rect.top - actualHeight - 8;
      tooltip.style.top = `${Math.max(8, Math.min(top, window.innerHeight - actualHeight - 8))}px`;
      tooltip.scrollTop = scrollTop;
    };
    const scroll = (event: Event) => { if (!(event.target instanceof Node) || !panel.current?.contains(event.target)) position(); };
    position();
    window.addEventListener("resize", position);
    window.addEventListener("scroll", scroll, true);
    return () => { window.removeEventListener("resize", position); window.removeEventListener("scroll", scroll, true); };
  });
  return <div ref={strip} className="drive-details" role="group" aria-label={t("Drive details")} onMouseLeave={leave} onMouseEnter={enter}
    onBlur={(event) => {
      if (!event.currentTarget.contains(event.relatedTarget as Node | null)) {
        focusedTopic.current = undefined;
        if (!hovered.current) onSelect(undefined);
      }
    }}>
    {visibleItems.map((entry) => <button key={entry.key} type="button" data-topic={entry.key}
      className={`drive-detail-icon${entry.tone === "warning" || entry.tone === "danger" ? " attention" : ""}`} data-tone={entry.tone || "info"}
      aria-label={t(entry.title)} aria-describedby={topic === entry.key ? `${id}-tooltip` : undefined}
      onMouseEnter={() => { cancelLeave(); onSelect({ connectionID: c.id, topic: entry.key }); }}
      onFocus={() => { focusedTopic.current = entry.key; cancelLeave(); onSelect({ connectionID: c.id, topic: entry.key }); }}
      onKeyDown={(event) => {
        if (topic !== entry.key || !panel.current) return;
        const directions: Record<string, number> = { ArrowDown: 40, ArrowUp: -40, PageDown: panel.current.clientHeight, PageUp: -panel.current.clientHeight, Home: -panel.current.scrollHeight, End: panel.current.scrollHeight };
        const direction = directions[event.key];
        if (direction !== undefined) { event.preventDefault(); panel.current.scrollTop += direction; }
      }}
      onClick={() => { cancelLeave(); onSelect({ connectionID: c.id, topic: entry.key }); }}>
      <svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d={icons[entry.key]} />
        {entry.key === "security" && security === "encrypted" && <path d="m8 12 3 3 5-6" />}
        {entry.key === "security" && security === "unencrypted" && <path className="security-off-mark" d="m3 3 18 18" />}
      </svg>
      {(entry.tone === "warning" || entry.tone === "danger") && <span className="detail-attention" aria-hidden="true">{entry.tone === "danger" || entry.key === "uploads" && c.pending !== null ? "!" : "?"}</span>}
    </button>)}
    {item && createPortal(<div ref={panel} id={`${id}-tooltip`} role="tooltip" className="drive-tooltip"
      onMouseEnter={enter} onMouseLeave={leave}>
      <strong>{t(item.title)}</strong>{item.content}
    </div>, document.body)}
  </div>;
}
