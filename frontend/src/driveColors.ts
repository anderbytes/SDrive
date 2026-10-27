import type { CSSProperties } from "react";
import palette from "../../internal/core/drive_palette.json";

export const driveColors = palette as [string, string, string][];

export function driveColorStyle(style = ""): CSSProperties {
  const color = driveColors.find(([id]) => id === style)?.[2];
  return {
    "--drive-background": color ? `color-mix(in srgb, ${color} 10%, var(--surface))` : "var(--default-drive-background)",
    "--drive-foreground": "var(--text)",
    "--drive-stripe": color ? `color-mix(in srgb, ${color} 65%, var(--text))` : "var(--default-drive-stripe)",
  } as CSSProperties;
}
