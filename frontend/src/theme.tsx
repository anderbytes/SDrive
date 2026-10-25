import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

export type Theme = "system" | "light" | "dark";
const Appearance = createContext({
  setTheme: (_: Theme) => {},
  setPreview: (_: Theme | undefined) => {},
});

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>("system");
  const [preview, setPreview] = useState<Theme>();
  useEffect(() => {
    const preference = preview ?? theme;
    const system = window.matchMedia("(prefers-color-scheme: dark)");
    const apply = () => { document.documentElement.dataset.theme = preference === "system" ? (system.matches ? "dark" : "light") : preference; };
    apply();
    system.addEventListener("change", apply);
    return () => system.removeEventListener("change", apply);
  }, [theme, preview]);
  return <Appearance.Provider value={{ setTheme, setPreview }}>{children}</Appearance.Provider>;
}

export const useTheme = () => useContext(Appearance);
