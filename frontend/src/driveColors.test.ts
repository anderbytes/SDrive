import { expect, test } from "vitest";
import { driveColors, driveColorStyle } from "./driveColors";

test("the fixed palette uses subtle tinting and ignores arbitrary colors", () => {
  expect(driveColors).toHaveLength(16);
  expect(new Set(driveColors.map(([id]) => id)).size).toBe(16);
  for (const [id, , color] of driveColors) {
    const style = driveColorStyle(id);
    expect(style).toHaveProperty("--drive-foreground", "var(--text)");
    if (color) expect(style).toHaveProperty("--drive-background", `color-mix(in srgb, ${color} 10%, var(--surface))`);
  }
  for (const invalid of ["#ff0000", "red; color:white", "url(example)"]) {
    expect(driveColorStyle(invalid)).toEqual(driveColorStyle(""));
  }
});
