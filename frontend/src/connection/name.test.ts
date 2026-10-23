import { expect, test } from "vitest";
import { driveNameError, trimDriveName } from "./name";

test("drive names count code points after Go-compatible whitespace trimming", () => {
  for (const unit of ["a", "é", "😀", "e\u0301", "👩‍💻"]) {
    const eighty = Array.from(unit.repeat(80)).slice(0, 80).join("");
    expect(driveNameError(` \u0085${eighty}\u3000 `)).toBe("");
    expect(driveNameError(eighty + "a")).toContain("80 characters");
  }
  expect(trimDriveName("\uFEFFDrive\uFEFF")).toBe("\uFEFFDrive\uFEFF");
  expect(driveNameError(" \u0085 ")).toContain("80 characters");
  expect(driveNameError(" \u0085 ", "Provider")).toBe("");
  expect(driveNameError("a\nb")).toContain("line breaks");
});
