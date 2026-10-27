import { useState } from "react";
import { afterEach, expect, test } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { DriveDetails, type DriveDetailSelection } from "./DriveDetails";
import { LanguageProvider } from "./i18n";
import type { ConnectionView } from "./api";
import fixtures from "./bridge-fixtures.json";

const connected: ConnectionView = {
  ...fixtures.samples.connectionUnknown, operation: "", mounted: true, recovery: false, pending: 0, error: "",
  uploads: { active: 0, queued: 0, cacheErrors: 0 },
  cache: { checkedAt: "2026-10-27T12:00:00Z", usedBytes: 0, pendingBytes: 0, error: "" },
  capacity: { state: "available", total: 100, used: 20, free: 80, checkedAt: "2026-10-27T12:00:00Z" },
  transportSecurity: { state: "encrypted", protocol: "https", source: "configuration", checkedAt: "2026-10-27T12:00:00Z" },
};

function DetailsHost({ connection }: { connection: ConnectionView }) {
  const [selection, setSelection] = useState<DriveDetailSelection>();
  return <LanguageProvider><DriveDetails connection={connection} provider={{ ...fixtures.samples.providerEmpty, fields: [] }}
    selection={selection} onSelect={setSelection} /></LanguageProvider>;
}
afterEach(cleanup);

test("network precedes capacity and separates measured values from unavailable categories", () => {
  const network = { state: "active", partial: false, startedAt: "2026-10-27T12:00:00Z", checkedAt: "2026-10-27T12:00:03Z", sent: 2048, received: 4096, filesSent: 1024, filesReceived: 2048 };
  const view = render(<DetailsHost connection={{ ...connected, network }} />);
  const labels = screen.getAllByRole("button").map(button => button.getAttribute("aria-label"));
  expect(labels.indexOf("Network") + 1).toBe(labels.indexOf("Remote capacity"));
  fireEvent.mouseEnter(screen.getByRole("button", { name: "Network" }));
  expect(screen.getByRole("table", { name: "Network usage by category" })).toBeDefined();
  expect(screen.getByRole("row", { name: /File contents/ }).textContent).toContain("1.0 KiB");
  expect(screen.getAllByText("Included in the measured total; separate value unavailable.")).toHaveLength(5);
  expect(screen.getByRole("row", { name: /Measured socket total/ }).textContent).toContain("4.0 KiB");
  expect(screen.getByRole("tooltip").textContent).toContain("Do not add the rows");
  act(() => screen.getByRole("button", { name: "Network" }).focus());
  fireEvent.keyDown(screen.getByRole("button", { name: "Network" }), { key: "ArrowDown" });
  expect(screen.getByRole("tooltip").scrollTop).toBe(40);
  view.rerender(<DetailsHost connection={{ ...connected, mounted: false, network: { ...network, state: "disconnected" } }} />);
  expect(screen.getByRole("button", { name: "Network" })).toBeDefined();
  expect(screen.getByRole("tooltip").textContent).toContain("Last connection retained");
});

test.each(["unavailable", "stale", "partial"])("network exposes %s without presenting missing counters as zero", state => {
  render(<DetailsHost connection={{ ...connected, network: { ...connected.network, state, partial: state === "partial" } }} />);
  const button = screen.getByRole("button", { name: "Network" });
  expect(button.getAttribute("data-tone")).toBe("warning");
  act(() => button.focus());
  expect(screen.getByRole("row", { name: /Measured socket total/ }).textContent).not.toContain("0 B");
  fireEvent.keyDown(button, { key: "Escape" });
  expect(screen.queryByRole("tooltip")).toBeNull();
});

test("idle connected and disconnected drives only expose meaningful details", () => {
  const view = render(<DetailsHost connection={connected} />);
  expect(screen.getAllByRole("button").map(button => button.getAttribute("aria-label"))).toEqual([
    "Remote destination", "Connection options", "Local cache", "Network", "Remote capacity", "Connection security",
  ]);
  view.rerender(<DetailsHost connection={{ ...connected, mounted: false }} />);
  expect(screen.getAllByRole("button").map(button => button.getAttribute("aria-label"))).toEqual(["Remote destination", "Connection options"]);
});

test.each([
  { name: "sending", pending: 1, active: 1, queued: 0, errors: 0, tone: "info", mark: undefined },
  { name: "queued", pending: 0, active: 0, queued: 2, errors: 0, tone: "warning", mark: "!" },
  { name: "pending", pending: 2, active: 0, queued: 0, errors: 0, tone: "warning", mark: "!" },
  { name: "unknown", pending: null, active: 1, queued: 0, errors: 0, tone: "warning", mark: "?" },
  { name: "failed", pending: null, active: 1, queued: 1, errors: 1, tone: "danger", mark: "!" },
])("uploads represent $name with the correct severity", ({ pending, active, queued, errors, tone, mark }) => {
  render(<DetailsHost connection={{ ...connected, pending, uploads: { active, queued, cacheErrors: errors } }} />);
  const uploads = screen.getByRole("button", { name: "Upload details" });
  expect(uploads.getAttribute("data-tone")).toBe(tone);
  expect(uploads.querySelector(".detail-attention")?.textContent).toBe(mark);
  if (pending !== null && !errors) expect(screen.queryByRole("button", { name: "Alerts and recovery" })).toBeNull();
});

test.each([
  { name: "nonempty", patch: { usedBytes: 1 }, tone: "info" },
  { name: "pending files", patch: { pendingBytes: 1 }, tone: "info" },
  { name: "failed", patch: { error: "Inspection failed" }, tone: "danger" },
])("disconnected cache stays visible when $name", ({ patch, tone }) => {
  render(<DetailsHost connection={{ ...connected, mounted: false, cache: { ...connected.cache, ...patch } }} />);
  expect(screen.getByRole("button", { name: "Local cache" }).getAttribute("data-tone")).toBe(tone);
});

test("unknown pending files remain visible without pretending the disconnected cache is clean", () => {
  render(<DetailsHost connection={{ ...connected, mounted: false, pending: null, cache: { ...connected.cache, checkedAt: null, usedBytes: null } }} />);
  for (const label of ["Local cache", "Upload details", "Alerts and recovery"]) {
    expect(screen.getByRole("button", { name: label }).getAttribute("data-tone")).toBe("warning");
  }
});

test.each(["available", "unavailable", "", "unsupported"])("remote capacity handles %s distinctly", (state) => {
  render(<DetailsHost connection={{ ...connected, capacity: { ...connected.capacity, state } }} />);
  const capacity = screen.queryByRole("button", { name: "Remote capacity" });
  if (state === "unsupported") expect(capacity).toBeNull();
  else expect(capacity?.getAttribute("data-tone")).toBe(state === "available" ? "info" : "warning");
});

test.each([
  { state: "encrypted", tone: "success", text: "Traffic encrypted", alerts: null },
  { state: "unencrypted", tone: "inactive", text: "Traffic unencrypted", alerts: "danger" },
  { state: "unknown", tone: "warning", text: "Traffic encryption could not be verified", alerts: "warning" },
])("security represents $state without disabling access to its explanation", ({ state, tone, text, alerts }) => {
  render(<DetailsHost connection={{ ...connected, transportSecurity: { ...connected.transportSecurity, state } }} />);
  const shield = screen.getByRole("button", { name: "Connection security" });
  expect(shield.matches(":disabled")).toBe(false);
  expect(shield.getAttribute("data-tone")).toBe(tone);
  expect(!!shield.querySelector(".security-off-mark")).toBe(state === "unencrypted");
  act(() => shield.focus());
  expect(screen.getByRole("tooltip").textContent).toContain(text);
  if (alerts) expect(screen.getByRole("button", { name: "Alerts and recovery" }).getAttribute("data-tone")).toBe(alerts);
  else expect(screen.queryByRole("button", { name: "Alerts and recovery" })).toBeNull();
});

test("recovery and errors outrank unknown-state warnings", () => {
  const view = render(<DetailsHost connection={{ ...connected, mounted: false, recovery: true, pending: null }} />);
  expect(screen.getByRole("button", { name: "Alerts and recovery" }).getAttribute("data-tone")).toBe("danger");
  expect(screen.getByRole("button", { name: "Local cache" })).toBeDefined();
  view.rerender(<DetailsHost connection={{ ...connected, pending: null, error: "Connection failed" }} />);
  expect(screen.getByRole("button", { name: "Alerts and recovery" }).getAttribute("data-tone")).toBe("danger");
});

test("removing the focused icon closes its old tooltip and restores focus to a visible detail", () => {
  const view = render(<DetailsHost connection={{ ...connected, pending: 1 }} />);
  act(() => screen.getByRole("button", { name: "Upload details" }).focus());
  expect(screen.getByRole("tooltip").textContent).toContain("1 pending upload(s)");
  view.rerender(<DetailsHost connection={connected} />);
  expect(screen.queryByRole("button", { name: "Upload details" })).toBeNull();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Remote destination" }));
  expect(screen.getByRole("tooltip").textContent).not.toContain("pending upload(s)");
});

test("removing a hovered icon does not steal focus from another control", () => {
  const view = render(<DetailsHost connection={{ ...connected, pending: 1 }} />);
  fireEvent.mouseEnter(screen.getByRole("button", { name: "Upload details" }));
  expect(screen.getByRole("tooltip")).toBeDefined();
  view.rerender(<DetailsHost connection={connected} />);
  expect(screen.queryByRole("tooltip")).toBeNull();
  expect(document.activeElement).toBe(document.body);
});
