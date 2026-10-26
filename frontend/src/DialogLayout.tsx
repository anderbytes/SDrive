import type { ReactNode, RefObject } from "react";
import { useLanguage } from "./i18n";

export function DialogLayout({ dialogRef, title, titleID, children, footer, onClose, closeDisabled = false, closeLabel, eyebrow, centered = false, navigation }: {
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
  navigation?: ReactNode;
}) {
  const { t } = useLanguage();
  return <dialog ref={dialogRef} className={`modal ${centered ? "confirmation" : "drawer"}${navigation ? " with-navigation" : ""}`} aria-modal="true" aria-labelledby={titleID}>
    <header className="dialog-heading">
      <div>{eyebrow && <div className="eyebrow">{eyebrow}</div>}<h2 id={titleID}>{title}</h2></div>
      <button type="button" className="dialog-close" aria-label={closeLabel || t("Close dialog")} disabled={closeDisabled} onClick={onClose}>×</button>
    </header>
    <div className="dialog-content">
      {navigation}
      <div className="dialog-body" onInvalidCapture={(event) => {
        let parent = (event.target as HTMLElement).parentElement;
        while (parent && parent !== dialogRef.current) {
          if (parent instanceof HTMLDetailsElement) parent.open = true;
          parent = parent.parentElement;
        }
      }}>{children}</div>
    </div>
    <div className="dialog-footer">{footer}</div>
  </dialog>;
}
