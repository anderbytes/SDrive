import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import type { ConnectionView, Provider } from "./api";
import { createPortal } from "react-dom";
import { useLanguage } from "./i18n";
import { errorMessage } from "./messages";
import { LoadingStatus } from "./Spinner";

export interface DriveFilters { search: string; states: string[]; providers: string[]; mounts: string[]; access: string[] }
export const emptyDriveFilters: DriveFilters = { search: "", states: [], providers: [], mounts: [], access: [] };

export function driveNeedsAttention(c: ConnectionView) {
  return !!(c.error || c.cache.error || c.uploads?.cacheErrors || c.recovery && !c.mounted || c.pending === null || c.pending > 0 || c.mounted && c.transportSecurity?.state !== "encrypted");
}

export function filterDrives(connections: ConnectionView[], filters: DriveFilters) {
  const query = filters.search.trim().toLocaleLowerCase();
  return connections.filter(c => (!query || [c.name, c.path, c.serverPath, c.mountFolder, c.letter && `${c.letter}:`].filter(Boolean).join(" ").toLocaleLowerCase().includes(query)) &&
    (!filters.states.length || filters.states.some(state => state === "attention" ? driveNeedsAttention(c) : state === "busy" ? c.busy : state === "connected" ? c.mounted : !c.mounted)) &&
    (!filters.providers.length || filters.providers.includes(c.provider)) &&
    (!filters.mounts.length || filters.mounts.includes(c.mountFolder ? "folder" : c.mountMode === "local" ? "local" : "network")) &&
    (!filters.access.length || filters.access.includes(c.readOnly ? "read" : "write")));
}

// Hidden drives keep their slots while the visible subsequence is reordered.
export function moveVisibleDrive(all: string[], visible: string[], from: string, to: string) {
  const next = [...visible];
  const start = next.indexOf(from), end = next.indexOf(to);
  if (start < 0 || end < 0) return all;
  next.splice(end, 0, next.splice(start, 1)[0]);
  const selected = new Set(visible);
  let i = 0;
  return all.map(id => selected.has(id) ? next[i++] : id);
}

export function DriveList({ connections, providers, disabled, onReorder, toolbarTarget, children }: {
  connections: ConnectionView[]; providers: Provider[]; disabled: boolean;
  onReorder: (ids: string[]) => Promise<void>; children: (c: ConnectionView) => ReactNode;
  toolbarTarget?: HTMLElement | null;
}) {
  const { t } = useLanguage();
  const helpID = useId();
  const filtersID = useId();
  const [filters, setFilters] = useState(emptyDriveFilters);
  const [filterOpen, setFilterOpen] = useState(false);
  const [order, setOrder] = useState<string[]>();
  const [moving, setMoving] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const draft = useRef<string[] | undefined>(undefined);
  const dragged = useRef("");
  const pointerID = useRef<number | undefined>(undefined);
  const [pointerPressed, setPointerPressed] = useState(false);
  const pointer = useRef({ id: "", startX: 0, startY: 0, x: 0, y: 0, offset: 0 });
  const slots = useRef(new Map<string, HTMLDivElement>());
  const positions = useRef(new Map<string, number>());
  const handles = useRef(new Map<string, HTMLButtonElement>());
  const restoreFocus = useRef("");
  const base = connections.map(c => c.id);
  const current = order && order.length === base.length && order.every(id => base.includes(id)) ? order : base;
  const byID = new Map(connections.map(c => [c.id, c]));
  const visible = filterDrives(current.map(id => byID.get(id)!), filters);
  const visibleIDs = visible.map(c => c.id);
  const count = Number(!!filters.search.trim()) + filters.states.length + filters.providers.length + filters.mounts.length + filters.access.length;
  const rememberPositions = () => {
    positions.current = new Map([...slots.current].map(([id, slot]) => [id, slot.firstElementChild!.getBoundingClientRect().top]));
  };
  const begin = (id: string) => {
    dragged.current = id; draft.current = [...base];
    setMoving(id); setOrder([...base]); setError("");
    setAnnouncement(t("Drive picked up. Use arrow keys to move, Space to save, or Escape to cancel."));
  };
  const move = (target: string) => {
    if (!draft.current || !dragged.current || target === dragged.current) return;
    rememberPositions();
    const ids = filterDrives(draft.current.map(id => byID.get(id)!).filter(Boolean), filters).map(c => c.id);
    draft.current = moveVisibleDrive(draft.current, ids, dragged.current, target);
    setOrder(draft.current);
    setAnnouncement(t("Drive moved to position {position}.", { position: draft.current.indexOf(dragged.current) + 1 }));
  };
  const finish = async (cancel = false) => {
    if (!dragged.current) return;
    const ids = draft.current;
    const id = dragged.current;
    rememberPositions();
    restoreFocus.current = id;
    dragged.current = ""; draft.current = undefined; pointerID.current = undefined; setPointerPressed(false); setMoving("");
    if (cancel || !ids || ids.every((id, index) => id === base[index])) { setOrder(undefined); return; }
    setSaving(true);
    try { await onReorder(ids); setAnnouncement(t("Drive order saved.")); }
    catch (e) { setError(t(errorMessage(e))); }
    finally { rememberPositions(); restoreFocus.current = id; setSaving(false); setOrder(undefined); }
  };
  useLayoutEffect(() => {
    const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;
    for (const [id, slot] of slots.current) {
      const entry = slot.firstElementChild as HTMLElement;
      const top = slot.getBoundingClientRect().top;
      const previous = positions.current.get(id);
      if (previous === undefined && !(id === dragged.current && pointerID.current !== undefined)) continue;
      entry.getAnimations?.().forEach(animation => animation.cancel());
      if (id === dragged.current && pointerID.current !== undefined) {
        entry.style.transform = `translateY(${pointer.current.y - pointer.current.offset - top}px)`;
      } else {
        entry.style.transform = "";
        if (previous !== undefined && Math.abs(previous - top) > 0.5 && !reducedMotion) {
          entry.animate?.([{ transform: `translateY(${previous - top}px)` }, { transform: "translateY(0)" }], { duration: 180, easing: "ease-out" });
        }
      }
    }
    positions.current.clear();
    const id = moving || restoreFocus.current;
    if (id && pointerID.current === undefined) handles.current.get(id)?.focus();
    restoreFocus.current = "";
  }, [moving, order, saving]);
  useEffect(() => {
    if (!pointerPressed) return;
    let frame: number;
    const insideList = () => {
      const bounds = [...slots.current.values()].map(slot => slot.getBoundingClientRect());
      return bounds.some(rect => pointer.current.x >= rect.left && pointer.current.x <= rect.right) &&
        pointer.current.y >= Math.min(...bounds.map(rect => rect.top)) && pointer.current.y <= Math.max(...bounds.map(rect => rect.bottom));
    };
    const updatePosition = () => {
      if (dragged.current) {
        const gesture = pointer.current;
        if (insideList()) {
          const edge = 48;
          const scroll = gesture.y < edge ? -Math.ceil((edge - gesture.y) / 4)
            : gesture.y > window.innerHeight - edge ? Math.ceil((gesture.y - window.innerHeight + edge) / 4) : 0;
          if (scroll) window.scrollBy(0, Math.max(-16, Math.min(16, scroll)));
          const ids = filterDrives((draft.current || []).map(id => byID.get(id)!).filter(Boolean), filters).map(c => c.id);
          const index = ids.indexOf(dragged.current);
          let target = index;
          for (let i = 0; i < ids.length; i++) {
            if (i === index) continue;
            const rect = slots.current.get(ids[i])?.getBoundingClientRect();
            if (rect && (i < index ? gesture.y < rect.top + rect.height / 2 : gesture.y > rect.top + rect.height / 2)) {
              target = i < index ? Math.min(target, i) : Math.max(target, i);
            }
          }
          if (target !== index) move(ids[target]);
        }
        const slot = slots.current.get(dragged.current);
        if (slot) (slot.firstElementChild as HTMLElement).style.transform = `translateY(${gesture.y - gesture.offset - slot.getBoundingClientRect().top}px)`;
      }
    };
    const track = () => { updatePosition(); frame = requestAnimationFrame(track); };
    const pointerMove = (event: PointerEvent) => {
      if (event.pointerId !== pointerID.current) return;
      pointer.current.x = event.clientX; pointer.current.y = event.clientY;
      if (!dragged.current && Math.hypot(event.clientX - pointer.current.startX, event.clientY - pointer.current.startY) >= 4) begin(pointer.current.id);
    };
    const release = (cancel: boolean) => {
      if (dragged.current) void finish(cancel);
      else { pointerID.current = undefined; setPointerPressed(false); }
    };
    const pointerUp = (event: PointerEvent) => {
      if (event.pointerId !== pointerID.current) return;
      pointer.current.x = event.clientX; pointer.current.y = event.clientY;
      updatePosition();
      release(!insideList());
    };
    const pointerCancel = (event: PointerEvent) => { if (event.pointerId === pointerID.current) release(true); };
    const blur = () => release(true);
    frame = requestAnimationFrame(track);
    window.addEventListener("pointermove", pointerMove);
    window.addEventListener("pointerup", pointerUp);
    window.addEventListener("pointercancel", pointerCancel);
    window.addEventListener("blur", blur);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("pointermove", pointerMove);
      window.removeEventListener("pointerup", pointerUp);
      window.removeEventListener("pointercancel", pointerCancel);
      window.removeEventListener("blur", blur);
    };
  }, [pointerPressed, connections, filters]);
  const toolbar = <div className="drive-list-toolbar">
    {filterOpen && <DriveFilterFields id={filtersID} filters={filters} disabled={saving || !!moving || pointerPressed}
      providers={providers.filter(p => connections.some(c => c.provider === p.id))} onChange={setFilters} />}
    <div className="drive-filter-actions">
      <button type="button" className={`drive-filter-toggle${count ? " active" : ""}`} title={t("Filter drives")}
        aria-label={t("Filter drives")} aria-expanded={filterOpen} aria-controls={filterOpen ? filtersID : undefined}
        disabled={saving || !!moving || pointerPressed} onClick={() => setFilterOpen(!filterOpen)}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M3 4h18l-7 8v7l-4 2v-9Z" /></svg>
        {count > 0 && <span className="drive-filter-count" aria-hidden="true">{count}</span>}
      </button>
      <span role="status">{t("Showing {visible} of {total} drives", { visible: visible.length, total: connections.length })}</span>
    </div>
  </div>;
  return <>
    {toolbarTarget === undefined ? toolbar : toolbarTarget && createPortal(toolbar, toolbarTarget)}
    <p id={helpID} className="sr-only">{t("Drag to reorder. With the keyboard, press Space, use arrow keys, then Space to save or Escape to cancel.")}</p>
    <p className="sr-only" role="status" aria-live="polite">{announcement}</p>
    {error && <p role="alert">{error}</p>}
    {saving && <LoadingStatus>{t("Saving drive order…")}</LoadingStatus>}
    {!visible.length && connections.length > 0 && <p role="status">{t("No drives match your filters.")}</p>}
    {visible.map((c, index) => <div key={c.id} className="drive-list-slot" data-drive-id={c.id}
      ref={element => { if (element) slots.current.set(c.id, element); else slots.current.delete(c.id); }}>
      <div className={`drive-list-entry${moving === c.id ? " moving" : ""}`}>
      <button ref={element => { if (element) handles.current.set(c.id, element); else handles.current.delete(c.id); }}
        type="button" className="drive-drag-handle" disabled={disabled || saving || connections.length < 2}
        aria-label={t("Reorder {name}", { name: c.name })} aria-describedby={helpID} aria-pressed={moving === c.id}
        onPointerDown={event => {
          if (event.button !== 0 || moving || pointerID.current !== undefined) return;
          event.preventDefault(); event.currentTarget.focus();
          pointerID.current = event.pointerId;
          pointer.current = { id: c.id, startX: event.clientX, startY: event.clientY, x: event.clientX, y: event.clientY,
            offset: event.clientY - slots.current.get(c.id)!.getBoundingClientRect().top };
          setPointerPressed(true);
        }}
        onKeyDown={event => {
          if (event.key === " " || event.key === "Enter") {
            event.preventDefault(); if (moving === c.id) void finish(); else if (!moving) begin(c.id);
          } else if (moving === c.id && event.key === "Escape") {
            event.preventDefault(); void finish(true);
          } else if (moving === c.id && ["ArrowUp", "ArrowDown"].includes(event.key)) {
            event.preventDefault(); const next = visibleIDs[index + (event.key === "ArrowUp" ? -1 : 1)]; if (next) move(next);
          } else if (moving === c.id && event.key === "Tab") void finish(true);
        }}>
        <svg viewBox="0 0 24 24" aria-hidden="true"><path d="M8 5h1m6 0h1M8 12h1m6 0h1M8 19h1m6 0h1" /></svg>
      </button>
      {children(c)}
      </div>
    </div>)}
  </>;
}

function DriveFilterFields({ id, filters, providers, disabled, onChange }: {
  id: string; filters: DriveFilters; providers: Provider[]; disabled: boolean; onChange: (filters: DriveFilters) => void;
}) {
  const { t } = useLanguage();
  const root = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const closeOutside = (event: PointerEvent) => root.current?.querySelectorAll("details[open]").forEach(details => {
      if (!details.contains(event.target as Node)) details.removeAttribute("open");
    });
    document.addEventListener("pointerdown", closeOutside);
    return () => document.removeEventListener("pointerdown", closeOutside);
  }, []);
  const groups = [
    { key: "states" as const, label: "Status", options: [["connected", "Connected"], ["disconnected", "Disconnected"], ["busy", "Operation in progress…"], ["attention", "Needs attention"]] },
    { key: "providers" as const, label: "Connection Type", options: providers.map(p => [p.id, p.name]) },
    { key: "mounts" as const, label: "Drive presentation", options: [["network", "Network drive"], ["local", "Local disk"], ["folder", "Local folder"]] },
    { key: "access" as const, label: "Access type", options: [["read", "Read only"], ["write", "Read and write"]] },
  ];
  return <div ref={root} id={id} className="drive-filter-fields" onKeyDown={event => {
    if (event.key !== "Escape") return;
    const details = (event.target as HTMLElement).closest("details[open]");
    if (details) { event.preventDefault(); details.removeAttribute("open"); details.querySelector("summary")?.focus(); }
  }}>
    <input type="search" aria-label={t("Search drives")} placeholder={t("Search drives")} disabled={disabled}
      value={filters.search} onChange={event => onChange({ ...filters, search: event.target.value })} />
    {groups.map(group => <details className="drive-filter-group" name={`${id}-category`} key={group.key}>
      <summary aria-disabled={disabled} onClick={event => { if (disabled) event.preventDefault(); }}>
        {t(group.label)}{filters[group.key].length > 0 && ` (${filters[group.key].length})`}
      </summary>
      <fieldset disabled={disabled}><legend className="sr-only">{t(group.label)}</legend>
        {group.options.map(([value, label]) => <label className="check" key={value}><input type="checkbox" checked={filters[group.key].includes(value)}
          onChange={event => onChange({ ...filters, [group.key]: event.target.checked ? [...filters[group.key], value] : filters[group.key].filter(item => item !== value) })} />{t(label)}</label>)}
      </fieldset>
    </details>)}
    <button type="button" disabled={disabled} onClick={() => onChange(emptyDriveFilters)}>{t("Clear filters")}</button>
  </div>;
}
