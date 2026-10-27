import { afterEach, expect, test, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { DriveList, emptyDriveFilters, filterDrives, moveVisibleDrive } from "./DriveList";
import { LanguageProvider } from "./i18n";
import type { ConnectionView } from "./api";
import fixtures from "./bridge-fixtures.json";

Object.defineProperties(HTMLDialogElement.prototype, {
  showModal: { configurable: true, value() { this.setAttribute("open", ""); } },
  close: { configurable: true, value() { this.removeAttribute("open"); } },
});
afterEach(cleanup);
const connections: ConnectionView[] = [
  { ...fixtures.samples.connectionUnknown, operation: "", id: "a", name: "Alpha", provider: "sftp", pending: 0, readOnly: true, mountMode: "network", serverPath: "server.test/documents" },
  { ...fixtures.samples.connectionUnknown, operation: "", id: "b", name: "Beta", provider: "webdav", pending: 0, mounted: true, mountMode: "local", mountFolder: "C:\\Mounts\\Beta", transportSecurity: { state: "encrypted", protocol: "https", source: "configuration", checkedAt: null } },
  { ...fixtures.samples.connectionUnknown, operation: "", id: "c", name: "Gamma", provider: "sftp", pending: null, busy: true, mountMode: "local" },
];
const providers = [{ ...fixtures.samples.providerEmpty, fields: [], id: "sftp", name: "SFTP" }, { ...fixtures.samples.providerEmpty, fields: [], id: "webdav", name: "WebDAV" }];
function show(onReorder = vi.fn().mockResolvedValue(undefined)) {
  const view = render(<LanguageProvider><DriveList connections={connections} providers={providers} disabled={false} onReorder={onReorder}>{c => <article><h2>{c.name}</h2></article>}</DriveList></LanguageProvider>);
  return { ...view, onReorder };
}

test("filters combine categories and match destination without changing the underlying collection", () => {
  expect(filterDrives(connections, { ...emptyDriveFilters, providers: ["sftp"], states: ["attention", "disconnected"], mounts: ["local"] }).map(c => c.id)).toEqual(["c"]);
  expect(filterDrives(connections, { ...emptyDriveFilters, search: " SERVER.TEST ", access: ["read"] }).map(c => c.id)).toEqual(["a"]);
  expect(filterDrives(connections, { ...emptyDriveFilters, states: ["connected"], mounts: ["folder"], access: ["write"] }).map(c => c.id)).toEqual(["b"]);
  expect(connections.map(c => c.id)).toEqual(["a", "b", "c"]);
});

test("filtered moves preserve hidden slots in both directions", () => {
  expect(moveVisibleDrive(["a", "b", "c", "d", "e"], ["a", "c", "e"], "a", "e")).toEqual(["c", "b", "e", "d", "a"]);
  expect(moveVisibleDrive(["a", "b", "c", "d", "e"], ["a", "c", "e"], "e", "a")).toEqual(["e", "b", "a", "d", "c"]);
});

test("filter dialog applies, cancels, clears and resets on a fresh session", () => {
  const view = show();
  fireEvent.click(screen.getByRole("button", { name: "Filter drives" }));
  fireEvent.change(screen.getByLabelText("Search drives"), { target: { value: "Alpha" } });
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(screen.getAllByRole("article")).toHaveLength(3);
  fireEvent.click(screen.getByRole("button", { name: "Filter drives" }));
  expect((screen.getByLabelText("Search drives") as HTMLInputElement).value).toBe("");
  fireEvent.click(screen.getByLabelText("SFTP"));
  fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));
  expect(screen.getAllByRole("heading").map(h => h.textContent)).toEqual(["Alpha", "Gamma"]);
  expect(screen.getByText("Showing 2 of 3 drives")).toBeDefined();
  fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
  expect(screen.getAllByRole("article")).toHaveLength(3);
  view.unmount(); show();
  expect(screen.getByText("Showing 3 of 3 drives")).toBeDefined();
});

test("keyboard drag saves a filtered order and Escape cancels without saving", async () => {
  const { onReorder } = show();
  fireEvent.click(screen.getByRole("button", { name: "Filter drives" }));
  fireEvent.click(screen.getByLabelText("SFTP"));
  fireEvent.click(screen.getByRole("button", { name: "Apply filters" }));
  const handle = screen.getByRole("button", { name: "Reorder Alpha" });
  handle.focus();
  fireEvent.keyDown(handle, { key: " " });
  fireEvent.keyDown(handle, { key: "ArrowDown" });
  expect(screen.getAllByRole("heading").map(h => h.textContent)).toEqual(["Gamma", "Alpha"]);
  expect(document.activeElement).toBe(handle);
  fireEvent.keyDown(handle, { key: "Escape" });
  expect(onReorder).not.toHaveBeenCalled();
  expect(screen.getAllByRole("heading").map(h => h.textContent)).toEqual(["Alpha", "Gamma"]);
  fireEvent.keyDown(handle, { key: " " });
  fireEvent.keyDown(handle, { key: "ArrowDown" });
  fireEvent.keyDown(handle, { key: " " });
  await waitFor(() => expect(onReorder).toHaveBeenCalledWith(["c", "b", "a"]));
});

test("a failed order save restores the snapshot and explains the error", async () => {
  show(vi.fn().mockRejectedValue(new Error("The drive list changed. Try again.")));
  const handle = screen.getByRole("button", { name: "Reorder Alpha" });
  fireEvent.keyDown(handle, { key: " " }); fireEvent.keyDown(handle, { key: "ArrowDown" }); fireEvent.keyDown(handle, { key: " " });
  await screen.findByRole("alert");
  expect(screen.getAllByRole("heading").map(h => h.textContent)).toEqual(["Alpha", "Beta", "Gamma"]);
});
