import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { DialogNavigation } from "./DialogNavigation";

afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const items = ["General", "Files", "Network"].map((key) => ({ key, label: key, id: key + "-tab", panel: key + "-panel" }));
function Settings() {
  const [selected, select] = useState("General");
  return <DialogNavigation label="Settings" items={items} selected={selected} onSelect={select} />;
}

test("vertical sections navigate by keyboard and wrap without selecting disabled categories", () => {
  const select = vi.fn();
  render(<DialogNavigation label="Settings" items={items.map((item) => ({ ...item, disabled: item.key === "Files" }))} selected="General" onSelect={select} />);
  expect(screen.getByRole("tablist").getAttribute("aria-orientation")).toBe("vertical");
  fireEvent.keyDown(screen.getByRole("tab", { name: "General" }), { key: "ArrowDown" });
  expect(select).toHaveBeenLastCalledWith("Network");
  expect(document.activeElement).toBe(screen.getByRole("tab", { name: "Network" }));
  fireEvent.keyDown(document.activeElement!, { key: "ArrowDown" });
  expect(select).toHaveBeenLastCalledWith("General");
});

test("narrow navigation collapses after choosing a section and returns focus to its toggle", () => {
  vi.stubGlobal("matchMedia", () => ({ matches: true }));
  render(<Settings />);
  const toggle = screen.getByRole("button", { name: /Sections/ });
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  fireEvent.click(screen.getByRole("tab", { name: "Files" }));
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  expect(toggle.textContent).toContain("Files");
  expect(screen.getByRole("tab", { name: "Files" }).getAttribute("aria-selected")).toBe("true");
  expect(document.activeElement).toBe(toggle);
});

test("setup navigation announces the current step and cannot skip blocked steps", () => {
  const select = vi.fn();
  render(<DialogNavigation steps label="Setup" selected="General" onSelect={select}
    items={items.map((item) => ({ ...item, disabled: item.key !== "General" }))} />);
  expect(screen.queryByRole("tablist")).toBeNull();
  expect(screen.getByRole("navigation").querySelector('[aria-current="step"]')?.textContent).toContain("General");
  fireEvent.click(screen.getByRole("button", { name: "Files" }));
  expect(select).not.toHaveBeenCalled();
});
