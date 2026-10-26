import React from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { DriveAppearanceDialog, DriveIcon, normalizeDriveImage } from "./DriveAppearance";
import { api, type Connection } from "./api";
import { LanguageProvider } from "./i18n";

vi.mock("@wailsio/runtime", () => ({ Call: { ByName: vi.fn() } }));
const connection: Connection = { id: "drive", remote: "remote", name: "Documents", letter: "S", provider: "webdav", path: "docs", recovery: true, readOnly: false, autoMount: false, targetHash: "", cardStyle: "blue", iconImage: "data:image/png;base64,synthetic" };
Object.defineProperties(HTMLDialogElement.prototype, {
  showModal: { configurable: true, value() { this.setAttribute("open", ""); } },
  close: { configurable: true, value() { this.removeAttribute("open"); } },
});
beforeEach(() => { vi.spyOn(api, "updateDriveAppearance").mockResolvedValue(); });
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

test("appearance saves only the selected visual fields and cancel leaves the drive unchanged", async () => {
  const close = vi.fn(), saved = vi.fn();
  const { unmount } = render(<LanguageProvider><DriveAppearanceDialog connection={connection} unavailable={false} onClose={close} onSave={saved} /></LanguageProvider>);
  fireEvent.click(screen.getByLabelText("Violet"));
  fireEvent.click(screen.getByText("Remove image"));
  fireEvent.click(screen.getByText("Cancel"));
  expect(api.updateDriveAppearance).not.toHaveBeenCalled();
  expect(close).toHaveBeenCalledOnce();
  expect(connection.iconImage).toBe("data:image/png;base64,synthetic");
  unmount();
  render(<LanguageProvider><DriveAppearanceDialog connection={connection} unavailable={false} onClose={close} onSave={saved} /></LanguageProvider>);
  expect((screen.getByLabelText("Blue") as HTMLInputElement).checked).toBe(true);
  fireEvent.click(screen.getByLabelText("Teal"));
  fireEvent.click(screen.getByText("Save changes"));
  await waitFor(() => expect(saved).toHaveBeenCalledOnce());
  expect(api.updateDriveAppearance).toHaveBeenCalledWith("drive", { cardStyle: "teal", iconImage: connection.iconImage });
});

test("failed saves keep the preview open and operations disable saving", async () => {
  vi.mocked(api.updateDriveAppearance).mockRejectedValue(new Error("Save failed"));
  const close = vi.fn();
  const { rerender } = render(<LanguageProvider><DriveAppearanceDialog connection={connection} unavailable={false} onClose={close} onSave={vi.fn()} /></LanguageProvider>);
  fireEvent.click(screen.getByText("Save changes"));
  await screen.findByRole("alert");
  expect(close).not.toHaveBeenCalled();
  rerender(<LanguageProvider><DriveAppearanceDialog connection={connection} unavailable={true} onClose={close} onSave={vi.fn()} /></LanguageProvider>);
  expect((screen.getByText("Save changes") as HTMLButtonElement).disabled).toBe(true);
});

test("folder mounts use a folder icon and broken custom images fall back to it", () => {
  const { container, rerender } = render(<DriveIcon connection={{ mountFolder: "C:\\Mount", letter: "" }} />);
  expect(screen.getByRole("img", { name: "Local folder" })).toBeDefined();
  expect(container.querySelector("svg")).not.toBeNull();
  expect(container.textContent).not.toContain(":");
  rerender(<DriveIcon connection={{ mountFolder: "C:\\Mount", letter: "", iconImage: connection.iconImage }} />);
  expect(container.querySelector("img")).not.toBeNull();
  fireEvent.error(container.querySelector("img")!);
  expect(container.querySelector("svg")).not.toBeNull();
});

test("image normalization rejects unsupported and oversized files before decoding", async () => {
  await expect(normalizeDriveImage(new File(["<svg/>"], "icon.svg", { type: "image/svg+xml" }))).rejects.toThrow("PNG, JPEG or WebP");
  await expect(normalizeDriveImage(new File([new Uint8Array(5 * 1024 ** 2 + 1)], "icon.png", { type: "image/png" }))).rejects.toThrow("5 MiB");
});
