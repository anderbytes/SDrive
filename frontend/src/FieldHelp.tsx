import { useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { useLanguage } from "./i18n";

export function FieldHelp({ id, label, children }: { id?: string; label: string; children: ReactNode }) {
  const generatedID = useId();
  const helpID = id || generatedID;
  const { t } = useLanguage();
  const root = useRef<HTMLDivElement>(null);
  const button = useRef<HTMLButtonElement>(null);
  const content = useRef<HTMLSpanElement>(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ left: 0, top: 0, maxWidth: 320 });
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const anchor = button.current!.getBoundingClientRect();
      const bounds = button.current!.closest("dialog")?.getBoundingClientRect();
      const leftEdge = Math.max(8, (bounds?.left || 0) + 8);
      const rightEdge = Math.min(innerWidth - 8, (bounds?.right || innerWidth) - 8);
      const width = Math.max(0, Math.min(320, rightEdge - leftEdge));
      const height = content.current!.getBoundingClientRect().height;
      const bottom = Math.min(innerHeight - 8, (bounds?.bottom || innerHeight) - 8);
      setPosition({ left: Math.max(leftEdge, Math.min(anchor.left, rightEdge - width)),
        top: Math.max((bounds?.top || 0) + 8, anchor.bottom + height + 8 <= bottom ? anchor.bottom + 4 : anchor.top - height - 4), maxWidth: width });
    };
    place();
    window.addEventListener("resize", place);
    document.addEventListener("scroll", place, true);
    return () => { window.removeEventListener("resize", place); document.removeEventListener("scroll", place, true); };
  }, [open]);
  useEffect(() => {
    if (!open) return;
    const outside = (event: PointerEvent) => { if (!root.current?.contains(event.target as Node)) setOpen(false); };
    const escape = (event: KeyboardEvent) => {
      if (event.key !== "Escape") return;
      event.preventDefault();
      event.stopPropagation();
      setOpen(false);
    };
    document.addEventListener("pointerdown", outside);
    document.addEventListener("keydown", escape, true);
    return () => { document.removeEventListener("pointerdown", outside); document.removeEventListener("keydown", escape, true); };
  }, [open]);
  return <div ref={root} className="field-help" onMouseEnter={() => setOpen(true)}
    onMouseLeave={() => { if (!root.current?.contains(document.activeElement)) setOpen(false); }}
    onBlur={(event) => { if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false); }}>
    <button ref={button} type="button" className="help-trigger" aria-label={t("Help for {field}", { field: label })}
      aria-describedby={helpID} onFocus={() => setOpen(true)} onClick={() => setOpen(true)}>ⓘ</button>
    <span ref={content} id={helpID} role={open ? "tooltip" : undefined} className={open ? "field-help-content" : "sr-only"}
      style={open ? position : undefined}>{children}</span>
  </div>;
}
