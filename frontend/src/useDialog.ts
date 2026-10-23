import { useEffect, useRef } from "react";

export function useDialog(close: () => void) {
  const ref = useRef<HTMLDialogElement>(null);
  const callback = useRef(close);
  callback.current = close;
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const dialog = ref.current;
    const cancel = (event: Event) => {
      // Keep asynchronous cleanup and busy guards in the owning dialog.
      event.preventDefault();
      callback.current();
    };
    dialog?.addEventListener("cancel", cancel);
    dialog?.showModal();
    return () => {
      dialog?.removeEventListener("cancel", cancel);
      dialog?.close();
      if (previous?.isConnected) previous.focus();
    };
  }, []);
  return ref;
}
