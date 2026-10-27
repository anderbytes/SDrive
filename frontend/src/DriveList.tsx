import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import type { ConnectionView, Provider } from "./api";
import { DialogLayout } from "./DialogLayout";
import { useDialog } from "./useDialog";
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

export function DriveList({ connections, providers, disabled, onReorder, children }: {
  connections: ConnectionView[]; providers: Provider[]; disabled: boolean;
  onReorder: (ids: string[]) => Promise<void>; children: (c: ConnectionView) => ReactNode;
}) {
  const { t } = useLanguage();
  const helpID = useId();
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
  const handles = useRef(new Map<string, HTMLButtonElement>());
  const restoreFocus = useRef("");
  const base = connections.map(c => c.id);
  const current = order && order.length === base.length && order.every(id => base.includes(id)) ? order : base;
  const byID = new Map(connections.map(c => [c.id, c]));
  const visible = filterDrives(current.map(id => byID.get(id)!), filters);
  const visibleIDs = visible.map(c => c.id);
  const count = Number(!!filters.search.trim()) + filters.states.length + filters.providers.length + filters.mounts.length + filters.access.length;
  const begin = (id: string) => {
    dragged.current = id; draft.current = [...base];
    setMoving(id); setOrder([...base]); setError("");
    setAnnouncement(t("Drive picked up. Use arrow keys to move, Space to save, or Escape to cancel."));
  };
  const move = (target: string) => {
    if (!draft.current || !dragged.current || target === dragged.current) return;
    const ids = filterDrives(draft.current.map(id => byID.get(id)!).filter(Boolean), filters).map(c => c.id);
    draft.current = moveVisibleDrive(draft.current, ids, dragged.current, target);
    setOrder(draft.current);
    setAnnouncement(t("Drive moved to position {position}.", { position: draft.current.indexOf(dragged.current) + 1 }));
  };
  const finish = async (cancel = false) => {
    if (!dragged.current) return;
    const ids = draft.current;
    const id = dragged.current;
    restoreFocus.current = id;
    dragged.current = ""; draft.current = undefined; pointerID.current = undefined; setMoving("");
    if (cancel || !ids || ids.every((id, index) => id === base[index])) { setOrder(undefined); return; }
    setSaving(true);
    try { await onReorder(ids); setAnnouncement(t("Drive order saved.")); }
    catch (e) { setError(t(errorMessage(e))); }
    finally { restoreFocus.current = id; setSaving(false); setOrder(undefined); }
  };
  useLayoutEffect(() => {
    const id = moving || restoreFocus.current;
    if (id && pointerID.current === undefined) handles.current.get(id)?.focus();
    restoreFocus.current = "";
  }, [moving, order, saving]);
  useEffect(() => {
    if (!moving) return;
    const rowAt = (event: PointerEvent) => document.elementFromPoint(event.clientX, event.clientY)?.closest<HTMLElement>("[data-drive-id]");
    const pointerMove = (event: PointerEvent) => {
      if (event.pointerId !== pointerID.current) return;
      const id = rowAt(event)?.dataset.driveId;
      if (id) move(id);
    };
    const pointerUp = (event: PointerEvent) => { if (event.pointerId === pointerID.current) void finish(!rowAt(event)); };
    const pointerCancel = (event: PointerEvent) => { if (event.pointerId === pointerID.current) void finish(true); };
    window.addEventListener("pointermove", pointerMove);
    window.addEventListener("pointerup", pointerUp);
    window.addEventListener("pointercancel", pointerCancel);
    return () => {
      window.removeEventListener("pointermove", pointerMove);
      window.removeEventListener("pointerup", pointerUp);
      window.removeEventListener("pointercancel", pointerCancel);
    };
  }, [moving, connections, filters]);
  return <>
    <div className="drive-list-toolbar">
      <button disabled={saving || !!moving} onClick={() => setFilterOpen(true)}>{t("Filter drives")}{count > 0 && ` (${count})`}</button>
      <span role="status">{t("Showing {visible} of {total} drives", { visible: visible.length, total: connections.length })}</span>
      {count > 0 && <button disabled={saving || !!moving} onClick={() => setFilters(emptyDriveFilters)}>{t("Clear filters")}</button>}
    </div>
    {filterOpen && <DriveFilterDialog filters={filters} providers={providers.filter(p => connections.some(c => c.provider === p.id))}
      onClose={() => setFilterOpen(false)} onApply={value => { setFilters(value); setFilterOpen(false); }} />}
    <p id={helpID} className="sr-only">{t("Drag to reorder. With the keyboard, press Space, use arrow keys, then Space to save or Escape to cancel.")}</p>
    <p className="sr-only" role="status" aria-live="polite">{announcement}</p>
    {error && <p role="alert">{error}</p>}
    {saving && <LoadingStatus>{t("Saving drive order…")}</LoadingStatus>}
    {!visible.length && connections.length > 0 && <p role="status">{t("No drives match your filters.")}</p>}
    {visible.map((c, index) => <div key={c.id} className={`drive-list-entry${moving === c.id ? " moving" : ""}`} data-drive-id={c.id}>
      <button ref={element => { if (element) handles.current.set(c.id, element); else handles.current.delete(c.id); }}
        type="button" className="drive-drag-handle" disabled={disabled || saving || connections.length < 2}
        aria-label={t("Reorder {name}", { name: c.name })} aria-describedby={helpID} aria-pressed={moving === c.id}
        onPointerDown={event => {
          if (event.button !== 0 || moving) return;
          event.preventDefault(); event.currentTarget.focus();
          begin(c.id); pointerID.current = event.pointerId;
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
    </div>)}
  </>;
}

function DriveFilterDialog({ filters, providers, onClose, onApply }: {
  filters: DriveFilters; providers: Provider[]; onClose: () => void; onApply: (filters: DriveFilters) => void;
}) {
  const { t } = useLanguage();
  const dialog = useDialog(onClose);
  const id = useId();
  const [draft, setDraft] = useState(filters);
  const groups = [
    { key: "states" as const, label: "Status", options: [["connected", "Connected"], ["disconnected", "Disconnected"], ["busy", "Operation in progress…"], ["attention", "Needs attention"]] },
    { key: "providers" as const, label: "Connection Type", options: providers.map(p => [p.id, p.name]) },
    { key: "mounts" as const, label: "Drive presentation", options: [["network", "Network drive"], ["local", "Local disk"], ["folder", "Local folder"]] },
    { key: "access" as const, label: "Access type", options: [["read", "Read only"], ["write", "Read and write"]] },
  ];
  return <DialogLayout dialogRef={dialog} titleID={id} title={t("Filter drives")} onClose={onClose}
    footer={<><button onClick={() => setDraft(emptyDriveFilters)}>{t("Clear filters")}</button><button onClick={onClose}>{t("Cancel")}</button><button className="primary" onClick={() => onApply(draft)}>{t("Apply filters")}</button></>}>
    <label>{t("Search drives")}<input type="search" value={draft.search} onChange={event => setDraft({ ...draft, search: event.target.value })} /></label>
    {groups.map(group => <fieldset className="option-group" key={group.key}><legend>{t(group.label)}</legend>
      {group.options.map(([value, label]) => <label className="check" key={value}><input type="checkbox" checked={draft[group.key].includes(value)}
        onChange={event => setDraft({ ...draft, [group.key]: event.target.checked ? [...draft[group.key], value] : draft[group.key].filter(item => item !== value) })} />{t(label)}</label>)}
    </fieldset>)}
  </DialogLayout>;
}
