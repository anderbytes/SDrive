export interface AppearancePreferences {
 textSize: "small" | "standard" | "large";
 hiddenCardDetails: number;
 capacityParts: number;
 sidebar: "expanded" | "collapsed" | "auto";
 motion: "full" | "reduced" | "off";
 disconnectSize: "compact" | "standard" | "large";
 disconnectDelayMs: number;
}
export const appearanceDefaults: AppearancePreferences = { textSize: "standard", hiddenCardDetails: 0, capacityParts: 7, sidebar: "expanded", motion: "full", disconnectSize: "standard", disconnectDelayMs: 1000 };
export function applyAppearance(value?: AppearancePreferences) {
 const p = value ?? appearanceDefaults;
 document.documentElement.style.setProperty("--text-scale", String({small: .9, standard: 1, large: 1.15}[p.textSize]));
 document.documentElement.dataset.motion = p.motion;
}
