import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { Events } from "@wailsio/runtime";
import { DisconnectOverlay, type DisconnectView } from "./DisconnectOverlay";

vi.mock("@wailsio/runtime", () => ({ Events: { On: vi.fn(), Emit: vi.fn() } }));
let receive: (event: { data: DisconnectView }) => void;
const unsubscribe = vi.fn();
beforeEach(() => {
  vi.clearAllMocks();
  Object.defineProperty(window, "matchMedia", { configurable: true, value: () => ({ matches: false, addEventListener() {}, removeEventListener() {} }) });
  vi.mocked(Events.On).mockImplementation((_name, callback) => {
    receive = callback as typeof receive;
    return unsubscribe;
  });
});
afterEach(cleanup);

function publish(revision: number, rows: DisconnectView["rows"], language = "en") {
  act(() => receive({ data: { revision, rows, language, theme: "dark" } }));
}

test("subscribes before requesting initial progress and ignores stale updates", () => {
  const view = render(<DisconnectOverlay />);
  expect(Events.On).toHaveBeenCalledWith("disconnect:state", expect.any(Function));
  expect(Events.Emit).toHaveBeenCalledWith("disconnect:ready");
  expect(vi.mocked(Events.On).mock.invocationCallOrder[0]).toBeLessThan(vi.mocked(Events.Emit).mock.invocationCallOrder[0]);
  publish(2, [{ name: "Storage", stage: "waitingUploads" }]);
  publish(1, [{ name: "Storage", stage: "succeeded" }]);
  expect(screen.getByText("Waiting for uploads…")).toBeDefined();
  expect(screen.queryByText("Drive disconnected.")).toBeNull();
  view.unmount();
  expect(unsubscribe).toHaveBeenCalled();
});

test("shows concurrent stages, success and persistent failure actions", () => {
  render(<DisconnectOverlay />);
  publish(1, [{ name: "One", stage: "checkingUploads" }, { name: "Two", stage: "unmounting" }]);
  expect(screen.getByRole("status").querySelectorAll(".spinner")).toHaveLength(2);
  expect(screen.getByText("Checking pending uploads…")).toBeDefined();
  expect(screen.getByText("Unmounting drive…")).toBeDefined();
  expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
  publish(2, [{ name: "One", stage: "succeeded" }, { name: "Two", stage: "failed" }]);
  expect(screen.getByRole("status").querySelectorAll(".spinner")).toHaveLength(0);
  expect(screen.getByText("Drive disconnected.")).toBeDefined();
  expect(screen.getByText("Attention required")).toBeDefined();
  fireEvent.click(screen.getByRole("button", { name: "Open SDrive" }));
  expect(Events.Emit).toHaveBeenCalledWith("disconnect:open");
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(Events.Emit).toHaveBeenCalledWith("disconnect:dismiss");
});

test("updates to generic localized status after locking", () => {
  render(<DisconnectOverlay />);
  publish(1, [{ name: "Private storage", stage: "waitingUploads" }]);
  publish(2, [{ name: "", stage: "waitingUploads" }], "pt-BR");
  expect(screen.queryByText("Private storage")).toBeNull();
  expect(screen.getByText("Unidade")).toBeDefined();
  expect(screen.getByText("Desconectando unidades…")).toBeDefined();
  expect(screen.getByRole("status").getAttribute("aria-live")).toBe("polite");
  publish(3, [{ name: "", stage: "succeeded" }], "pt-BR");
  expect(screen.getByText("Unidade desconectada.")).toBeDefined();
});
