import type { ReactNode, RefObject } from "react";
import { useLanguage } from "./i18n";

export function DialogLayout({ dialogRef, title, titleID, children, footer, onClose, closeDisabled = false, closeLabel, eyebrow, centered = false }: {
  dialogRef: RefObject<HTMLDialogElement | null>;
  title: string;
  titleID: string;
  children: ReactNode;
  footer: ReactNode;
  onClose: () => void;
  closeDisabled?: boolean;
  closeLabel?: string;
  eyebrow?: string;
  centered?: boolean;
}) {
  const { t } = useLanguage();
  return <dialog ref={dialogRef} className={`modal ${centered ? "confirmation" : "drawer"}`} aria-modal="true" aria-labelledby={titleID}>
    <header className="dialog-heading">
      <div>{eyebrow && <div className="eyebrow">{eyebrow}</div>}<h2 id={titleID}>{title}</h2></div>
      <button type="button" className="dialog-close" aria-label={closeLabel || t("Close dialog")} disabled={closeDisabled} onClick={onClose}>×</button>
    </header>
    <div className="dialog-body" onFocus={(event) => {
      if (event.target.getAttribute("role") === "tab") event.target.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    }}>{children}</div>
    <div className="dialog-footer">{footer}</div>
  </dialog>;
}
