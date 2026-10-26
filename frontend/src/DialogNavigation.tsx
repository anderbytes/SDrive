import { useId, useRef, useState } from "react";
import { useLanguage } from "./i18n";

export interface DialogSection {
  key: string;
  label: string;
  id?: string;
  panel?: string;
  disabled?: boolean;
  attention?: boolean;
}

export function DialogNavigation({ label, items, selected, onSelect, steps = false }: {
  label: string;
  items: DialogSection[];
  selected: string;
  onSelect: (key: string) => void;
  steps?: boolean;
}) {
  const { t } = useLanguage();
  const id = useId();
  const toggle = useRef<HTMLButtonElement>(null);
  const [expanded, setExpanded] = useState(false);
  const current = items.find((item) => item.key === selected);
  return <aside className={`dialog-navigation ${expanded ? "expanded" : ""}`}>
    <button ref={toggle} type="button" className="navigation-toggle" aria-expanded={expanded} aria-controls={id}
      onClick={() => setExpanded(!expanded)}>{t("Sections")} · {current?.label} <span aria-hidden="true">▾</span></button>
    <div id={id} className="navigation-items" role={steps ? "navigation" : "tablist"} aria-label={label}
      aria-orientation={steps ? undefined : "vertical"}>
      {items.map((item, index) => <button key={item.key} id={item.id} type="button"
        role={steps ? undefined : "tab"} aria-selected={steps ? undefined : selected === item.key}
        aria-current={steps && selected === item.key ? "step" : undefined} aria-controls={item.panel}
        tabIndex={steps || selected === item.key ? 0 : -1} disabled={item.disabled}
        className={item.attention ? "attention" : undefined}
        onClick={() => {
          onSelect(item.key);
          if (window.matchMedia("(max-width: 639px)").matches) {
            setExpanded(false);
            toggle.current?.focus();
          }
        }}
        onKeyDown={(event) => {
          if (steps || !["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
          event.preventDefault();
          const available = items.filter((entry) => !entry.disabled);
          const position = available.indexOf(items[index]);
          const next = event.key === "Home" ? available[0] : event.key === "End" ? available.at(-1)!
            : available[(position + (event.key === "ArrowDown" ? 1 : available.length - 1)) % available.length];
          onSelect(next.key);
          if (next.id) document.getElementById(next.id)?.focus();
        }}>
        {steps && <span className="step-number" aria-hidden="true">{index + 1}</span>}
        <span>{item.label}</span>{item.attention && <span aria-hidden="true"> !</span>}
      </button>)}
    </div>
  </aside>;
}
