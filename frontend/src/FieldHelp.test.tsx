import { afterEach, expect, test } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { FieldHelp } from "./FieldHelp";

afterEach(cleanup);

test("help keeps its accessible description and opens with hover, focus or click", () => {
  render(<FieldHelp id="server-help" label="Server">Use your server address.</FieldHelp>);
  const trigger = screen.getByRole("button", { name: "Help for Server" });
  expect(trigger.getAttribute("aria-describedby")).toBe("server-help");
  expect(document.getElementById("server-help")?.textContent).toBe("Use your server address.");
  expect(screen.queryByRole("tooltip")).toBeNull();
  fireEvent.mouseEnter(trigger);
  expect(screen.getByRole("tooltip")).toBeTruthy();
  fireEvent.mouseLeave(trigger);
  expect(screen.queryByRole("tooltip")).toBeNull();
  act(() => trigger.focus());
  expect(screen.getByRole("tooltip")).toBeTruthy();
  act(() => trigger.blur());
  expect(screen.queryByRole("tooltip")).toBeNull();
  fireEvent.click(trigger);
  expect(screen.getByRole("tooltip")).toBeTruthy();
  fireEvent.pointerDown(document.body);
  expect(screen.queryByRole("tooltip")).toBeNull();
});

test("Escape dismisses hovered help before it can close the dialog", () => {
  render(<FieldHelp label="Server">Use your server address.</FieldHelp>);
  fireEvent.mouseEnter(screen.getByRole("button"));
  const escape = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
  fireEvent(document.body, escape);
  expect(escape.defaultPrevented).toBe(true);
  expect(screen.queryByRole("tooltip")).toBeNull();
});
