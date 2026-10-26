import { useEffect, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import { LanguageProvider, isLanguage, useLanguage } from "./i18n";
import { ThemeProvider, useTheme, type Theme } from "./theme";

const stages = {
  checkingUploads: "Checking pending uploads…",
  waitingUploads: "Waiting for uploads…",
  unmounting: "Unmounting drive…",
  succeeded: "Drive disconnected.",
  failed: "Could not disconnect safely. Open SDrive to check uploads and recovery.",
} as const;

export interface DisconnectView {
  revision: number;
  language: string;
  theme: string;
  rows: { name: string; stage: keyof typeof stages }[];
}

export function DisconnectOverlay() {
  return <ThemeProvider><LanguageProvider><DisconnectStatus /></LanguageProvider></ThemeProvider>;
}

function DisconnectStatus() {
  const { t, setLanguage } = useLanguage();
  const { setTheme } = useTheme();
  const revision = useRef(0);
  const [view, setView] = useState<DisconnectView>();
  useEffect(() => {
    const unsubscribe = Events.On("disconnect:state", (event) => {
      const next = event.data as DisconnectView;
      if (next.revision <= revision.current) return;
      revision.current = next.revision;
      setLanguage(isLanguage(next.language) ? next.language : "en");
      setTheme((next.theme || "system") as Theme);
      setView(next);
    });
    void Events.Emit("disconnect:ready");
    return unsubscribe;
  }, []);
  const active = view?.rows.some(row => row.stage !== "failed" && row.stage !== "succeeded");
  const failed = view?.rows.some(row => row.stage === "failed");
  return <section className="disconnect-overlay" aria-label={t("Drive disconnection")}>
    <header><strong>SDrive</strong><span>{t(active ? "Disconnecting drives…" : failed ? "Attention required" : "Disconnection complete")}</span></header>
    <div className="disconnect-rows" role="status" aria-live="polite" aria-atomic="true" tabIndex={0}>
      {view?.rows.map((row, index) => <div className={`disconnect-row ${row.stage}`} key={index}>
        <span className="disconnect-indicator" aria-hidden="true">{row.stage === "succeeded" ? "✓" : row.stage === "failed" ? "!" : ""}</span>
        <div><strong>{row.name || t("Drive")}</strong><p>{t(stages[row.stage])}</p></div>
      </div>)}
    </div>
    {failed && <footer><button onClick={() => void Events.Emit("disconnect:open")}>{t("Open SDrive")}</button><button onClick={() => void Events.Emit("disconnect:dismiss")}>{t("Close")}</button></footer>}
  </section>;
}
