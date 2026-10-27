import type { ReactNode } from "react";

export function Spinner({ large = false }: { large?: boolean }) {
  return <span className={`spinner${large ? " spinner-large" : ""}`} aria-hidden="true" />;
}

export function LoadingStatus({ children, large = false }: { children: ReactNode; large?: boolean }) {
  return <p role="status" className={`loading-status${large ? " loading-status-large" : ""}`}>
    <Spinner large={large} /><span>{children}</span>
  </p>;
}
