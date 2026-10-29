import { createContext, useContext, useEffect, useState, type ReactNode } from "react";

export type Density = "compact" | "balanced" | "comfortable";
export const themeLabels = {
  system: "System", light: "Light", dark: "Dark",
  retro: "Retro 95", outline: "Outline Ink", terminal: "Phosphor Terminal",
  editorial: "Swiss Editorial", atelier: "Organic Atelier",
} as const;
export type Theme = keyof typeof themeLabels;
const Appearance = createContext({
  setDensity: (_: Density) => {},
  setDensityPreview: (_: Density | undefined) => {},
  setTheme: (_: Theme) => {},
  setPreview: (_: Theme | undefined) => {},
});

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [density, setDensity] = useState<Density>("balanced");
  const [densityPreview, setDensityPreview] = useState<Density>();
  useEffect(() => { document.documentElement.dataset.density = densityPreview ?? density; }, [density, densityPreview]);
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
  return <Appearance.Provider value={{ setTheme, setPreview, setDensity, setDensityPreview }}>{children}</Appearance.Provider>;
}

export const useTheme = () => useContext(Appearance);
