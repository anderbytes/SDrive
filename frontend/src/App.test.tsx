import React from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { App } from "./App";
import { ImportConfigurationDialog } from "./ConfigurationPortability";
import { api, type ConnectionView, type Snapshot } from "./api";
import bridgeFixtures from "./bridge-fixtures.json";
import { translate } from "./i18n";

// JSDOM has no modal top layer; native focus and isolation are checked in WebView2.
Object.defineProperties(HTMLDialogElement.prototype, {
  showModal: { configurable: true, value() { this.setAttribute("open", ""); } },
  close: { configurable: true, value() { this.removeAttribute("open"); } },
});
const selectAvailableCategory = () => {
  const select = screen.queryByLabelText("Connection type") || screen.queryByLabelText("Tipo de conexão");
  if (select instanceof HTMLSelectElement && !select.options.length) fireEvent.click(screen.getByRole("tab", { name: /^(Advanced|Avançados)$/ }));
};
const cancelDialog = () => fireEvent(screen.getByRole("dialog"), new Event("cancel", { cancelable: true }));

vi.mock("@wailsio/runtime", () => ({ Call: { ByName: vi.fn() } }));

vi.mock("./api", async (importOriginal) => ({
  ...await importOriginal<typeof import("./api")>(),
  api: {
    inspectFileAccess: vi.fn(), accessStatus: vi.fn(), configureAccess: vi.fn(), disableAccess: vi.fn(), lock: vi.fn(), unlockAccess: vi.fn(), recoverAccess: vi.fn(), onAccessChanged: vi.fn(),
    exportConfiguration: vi.fn(), previewImport: vi.fn(), commitImport: vi.fn(), removeImportDraft: vi.fn(),
    selectDirectory: vi.fn(), changeCacheLocation: vi.fn(), refreshDirectoryCache: vi.fn(),
    browsers: vi.fn(), proxy: vi.fn(), saveProxy: vi.fn(),
    about: vi.fn(),
    diagnostics: vi.fn(),
    copyDiagnostics: vi.fn(),
    desktopVisible: vi.fn(),
    onDesktopVisibility: vi.fn(),
    snapshot: vi.fn(),
    begin: vi.fn(),
    status: vi.fn(),
    end: vi.fn(),
    answer: vi.fn(),
    settings: vi.fn(),
    exit: vi.fn(),
    remove: vi.fn(),
    folders: vi.fn(),
    confirm: vi.fn(),
    mount: vi.fn(),
    test: vi.fn(),
    update: vi.fn(),
    unmount: vi.fn(),
    open: vi.fn(),
    fields: vi.fn(),
    selectFile: vi.fn(),
    probe: vi.fn(),
    prerequisite: vi.fn(),
    restart: vi.fn(),
    restoreSettings: vi.fn(),
    verifyRecovery: vi.fn(),
    retryCleanup: vi.fn(),
  } satisfies { [K in keyof typeof api]: ReturnType<typeof vi.fn<(typeof api)[K]>> },
}));
const connection: ConnectionView = {
  id: "1",
  remote: "2",
  name: "My Drive",
  provider: "drive",
  path: "",
  letter: "S",
  readOnly: false,
  autoMount: false,
  recovery: false,
  targetHash: "",
  transportSecurity: { state: "disconnected", protocol: "", source: "", checkedAt: null },
  mounted: false,
  busy: false,
  operation: "",
  pending: 0,
  uploads: null,
  capacity: { total: null, used: null, free: null, checkedAt: null, state: "" },
  cache: { checkedAt: null, pendingBytes: 0, usedBytes: 0, error: "" },
  error: "",
};
const access = { setupPending: false, enabled: false, locked: false, language: "en", policy: { idleMinutes: 15, disconnectIdle: false, reconnect: false }, retryAt: "0001-01-01T00:00:00Z", error: "", warning: "" };
const snapshot = {
  access,
  importDrafts: [], desktopWarning: "",
  cacheLocation: "C:\\Synthetic\\Cache",
  freeBytes: null,
  providers: [
    {
      id: "drive",
      name: "Google Drive",
      backend: "drive", transportEncryption: "required",
      experimental: false,
      fields: [],
      oauth: true,
      available: true,
    },
  ],
  connections: [] as ConnectionView[],
  settings: { language: "en" as const, autoStart: false, cacheGiB: 2, openExplorerAfterConnect: false, showServiceIcons: false },
  letters: ["S", "T"],
  winFsp: true,
  engineError: "",
  settingsError: "",
  backupAvailable: false,
  recoveryHold: false,
  maintenanceError: "",
  startupWarning: "",
};
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.accessStatus).mockResolvedValue(access);
  vi.mocked(api.onAccessChanged).mockReturnValue(() => {});
  vi.mocked(api.browsers).mockResolvedValue([{ id: "", name: "Default browser" }]);
  vi.mocked(api.proxy).mockResolvedValue({ address: "", username: "", password: "", hasPassword: false });
  vi.mocked(api.desktopVisible).mockResolvedValue(true);
  vi.mocked(api.onDesktopVisibility).mockReturnValue(() => {});
  vi.mocked(api.snapshot).mockResolvedValue(snapshot);
  vi.mocked(api.end).mockResolvedValue();
  vi.mocked(api.folders).mockResolvedValue([]);
  vi.mocked(api.confirm).mockResolvedValue("connected-id");
  vi.mocked(api.mount).mockResolvedValue();
});
afterEach(() => cleanup());

test("import preview requires conflict resolution and explicit preference selection", async () => {
  const document = { ...bridgeFixtures.samples.importPreview.document, preferences: { ...snapshot.settings }, connections: [{ ...bridgeFixtures.samples.importDraft.definition, name: "My Drive", provider: "drive", letter: "S", mountFolder: "", mountMode: "network", sessionOnly: false, autoMount: false }] };
  const onSaved = vi.fn().mockResolvedValue(undefined);
  vi.mocked(api.commitImport).mockResolvedValue();
  render(<ImportConfigurationDialog preview={{ id: "preview", document }} data={{ ...snapshot, connections: [connection] }} onClose={vi.fn()} onSaved={onSaved} />);
  expect(screen.getByRole("button", { name: "Save imported drafts" }).hasAttribute("disabled")).toBe(true);
  fireEvent.change(screen.getByLabelText("Drive name"), { target: { value: "Imported Drive" } });
  fireEvent.change(screen.getByLabelText("Windows drive letter"), { target: { value: "T" } });
  const preferences = screen.getByRole("checkbox", { name: "Apply imported language, cache timing and desktop preferences" });
  expect((preferences as HTMLInputElement).checked).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Save imported drafts" }));
  await waitFor(() => expect(onSaved).toHaveBeenCalledOnce());
  expect(api.commitImport).toHaveBeenCalledWith("preview", expect.objectContaining({ connections: [expect.objectContaining({ name: "Imported Drive", letter: "T" })] }), false);
  expect(api.mount).not.toHaveBeenCalled();
});

test("failed import remains reviewable and cancellation never saves", async () => {
  const document = { ...bridgeFixtures.samples.importPreview.document, preferences: { ...snapshot.settings }, connections: [{ ...bridgeFixtures.samples.importDraft.definition, name: "Imported", provider: "drive", letter: "T", mountFolder: "", mountMode: "network", sessionOnly: false, autoMount: false }] };
  const onClose = vi.fn();
  vi.mocked(api.commitImport).mockRejectedValue(new Error("Disk write failed"));
  render(<ImportConfigurationDialog preview={{ id: "preview", document }} data={snapshot} onClose={onClose} onSaved={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: "Save imported drafts" }));
  await screen.findByText("Disk write failed");
  expect(screen.getByLabelText("Drive name")).toHaveProperty("value", "Imported");
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(onClose).toHaveBeenCalledOnce();
  expect(api.commitImport).toHaveBeenCalledOnce();
});

test("imported drafts preserve the selected method and require the existing authentication wizard", async () => {
  const draft = { id: "draft", definition: { ...bridgeFixtures.samples.importDraft.definition, provider: "drive", name: "Imported Drive", letter: "T", mountFolder: "", authMethod: "oauth", sessionOnly: false, autoMount: false, values: { root_folder_id: "shared-root" } } };
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, importDrafts: [draft] });
  vi.mocked(api.begin).mockResolvedValue("session");
  vi.mocked(api.status).mockResolvedValue({ id: "session", busy: true, complete: false, authenticated: false, connectionID: "", question: null, error: "" });
  render(<App />);
  await screen.findByText("Authentication required for imported configuration");
  expect(api.mount).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Authenticate" }));
  expect(screen.getByRole("tab", { name: "Advanced" }).getAttribute("aria-selected")).toBe("true");
  expect(screen.getByLabelText("Drive name")).toHaveProperty("value", "Imported Drive");
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  fireEvent.click(screen.getByRole("button", { name: "Sign in with Google Drive" }));
  await waitFor(() => expect(api.begin).toHaveBeenCalledWith(expect.objectContaining({ importDraftID: "draft", values: { root_folder_id: "shared-root" }, connection: expect.objectContaining({ id: "", provider: "drive" }) })));
  expect(api.fields).not.toHaveBeenCalled();
});

test.each(["en", "pt-BR"] as const)("About shows component versions and local diagnostics in %s", async (language) => {
  const t = (source: string) => translate(language, source);
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language } });
  vi.mocked(api.about).mockResolvedValue(bridgeFixtures.samples.about);
  vi.mocked(api.diagnostics).mockResolvedValue("Synthetic diagnostic report");
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: t("About SDrive") }));
  const dialog = await screen.findByRole("dialog", { name: t("About SDrive") });
  await within(dialog).findByText(bridgeFixtures.samples.about.appVersion);
  const actions = within(dialog).getAllByRole("button").map((button) => button.getAttribute("aria-label") || button.textContent);
  expect(actions.every((name) => [t("Close"), t("Close dialog"), t("View diagnostics")].includes(name || ""))).toBe(true);
  fireEvent.click(within(dialog).getByRole("button", { name: t("View diagnostics") }));
  await within(dialog).findByDisplayValue("Synthetic diagnostic report");
  expect(within(dialog).getByRole("button", { name: t("Copy report") })).toBeDefined();
});

test("connection option indicators preserve recovery and upload messages", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, autoMount: true, readOnly: true, recovery: true, pending: 2, uploadLimit: 1024 }] });
  render(<App />);
  fireEvent.mouseEnter(await screen.findByRole("button", { name: "Connection options" }));
  await screen.findByText("Automatic mounting enabled");
  expect(screen.getByRole("list", { name: "Connection options" }).textContent).toContain("Upload limit: 1.0 KiB/s");
  expect(screen.getAllByText(/Recovery required/).length).toBeGreaterThan(0);
  fireEvent.mouseEnter(screen.getByRole("button", { name: "Alerts and recovery" }));
  expect(screen.getByText(/Recover using the original account and destination/)).toBeTruthy();
});

test("independent desktop failure messages remain localized together", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" }, desktopWarning: "Explorer drive names or service icons could not be updated. Existing customization may take precedence.\nThe drive connected, but Explorer could not be opened. Use Open in Explorer to retry." });
  render(<App />);
  await screen.findByText("Não foi possível atualizar os nomes das unidades ou os ícones de serviço do Explorer. Personalizações existentes podem ter prioridade. A unidade conectou, mas não foi possível abrir o Explorer. Use Abrir no Explorer para tentar novamente.");
});

test("Basic is initially selected and category tabs support keyboard navigation", async () => {
  const gcs = { ...snapshot.providers[0], id: "gcs", name: "Google Cloud Storage", backend: "google cloud storage" };
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [...snapshot.providers, ftpProvider, gcs] });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  const basic = screen.getByRole("tab", { name: "Basic" });
  expect(basic.getAttribute("aria-selected")).toBe("true");
  expect(screen.queryByRole("option", { name: "Google Drive" })).toBeNull();
  expect(screen.getByRole("option", { name: "Google Cloud Storage — service account" })).toBeDefined();
  fireEvent.keyDown(basic, { key: "ArrowRight" });
  const advanced = screen.getByRole("tab", { name: "Advanced" });
  expect(advanced.getAttribute("aria-selected")).toBe("true");
  expect(document.activeElement).toBe(advanced);
  expect(screen.getByText(/These connections require additional setup/)).toBeDefined();
  expect(screen.getByRole("option", { name: "Google Cloud Storage — OAuth" })).toBeDefined();
  expect(screen.queryByRole("option", { name: "FTP / FTPS" })).toBeNull();
  fireEvent.keyDown(advanced, { key: "Home" });
  expect(basic.getAttribute("aria-selected")).toBe("true");
  expect(document.activeElement).toBe(basic);
});

test("Connect on a session-only drive requests authentication instead of mounting", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, sessionOnly: true, authMethod: "oauth" }] });
  vi.mocked(api.fields).mockResolvedValue({ values: { root_folder_id: "synthetic-root" }, transport: "" });
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "Connect" }));
  await screen.findByRole("heading", { name: "Reconfigure account" });
  expect(api.mount).not.toHaveBeenCalled();
  expect((screen.getByLabelText("Connect automatically when SDrive starts") as HTMLInputElement).disabled).toBe(true);
});

test("network form converts KiB limits to independent byte rates", async () => {
  const webdav = { ...snapshot.providers[0], id: "webdav", name: "WebDAV", backend: "webdav", transportEncryption: "optional", oauth: false };
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [webdav], connections: [{ ...connection, provider: "webdav" }] });
  vi.mocked(api.update).mockResolvedValue();
  render(<App />);
  fireEvent.click(await screen.findByText("Edit drive"));
  fireEvent.click(screen.getByRole("tab", { name: "Network" }));
  fireEvent.change(screen.getByLabelText("Download limit (KiB/s)"), { target: { value: "512" } });
  fireEvent.change(screen.getByLabelText("Upload limit (KiB/s)"), { target: { value: "128" } });
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  await waitFor(() => expect(api.update).toHaveBeenCalledWith(expect.objectContaining({ downloadLimit: 512 * 1024, uploadLimit: 128 * 1024 })));
});

test("private browser selection reaches authentication without silently falling back", async () => {
  vi.mocked(api.browsers).mockResolvedValue([{ id: "", name: "Default browser" }, { id: "edge", name: "Microsoft Edge" }]);
  vi.mocked(api.begin).mockResolvedValue("private-draft");
  vi.mocked(api.status).mockResolvedValue({ ...bridgeFixtures.samples.sessionEmpty, id: "private-draft", busy: true });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  fireEvent.click(screen.getByRole("tab", { name: "Advanced" }));
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await screen.findByRole("option", { name: "Microsoft Edge" });
  fireEvent.change(screen.getByLabelText("Sign-in browser"), { target: { value: "edge" } });
  fireEvent.click(screen.getByLabelText("Open a private sign-in window"));
  fireEvent.click(screen.getByRole("button", { name: "Sign in with Google Drive" }));
  await waitFor(() => expect(api.begin).toHaveBeenCalledWith(expect.objectContaining({ connection: expect.objectContaining({ browser: "edge", privateBrowser: true }) })));
});

test("new drive accepts eighty emoji code points and blocks eighty-one without truncating", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({ ...bridgeFixtures.samples.sessionEmpty, id: "draft", busy: true });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  const input = screen.getByLabelText("Drive name") as HTMLInputElement;
  const tooLong = "😀".repeat(81);
  fireEvent.change(input, { target: { value: tooLong } });
  fireEvent.click(screen.getByText("Continue"));
  const error = await screen.findByText("name must contain between 1 and 80 characters");
  expect(input.value).toBe(tooLong);
  expect(input.getAttribute("aria-describedby")).toBe(error.id);
  expect(api.begin).not.toHaveBeenCalled();
  const valid = "😀".repeat(80);
  fireEvent.change(input, { target: { value: ` \u0085${valid}\u3000 ` } });
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  await waitFor(() => expect(api.begin).toHaveBeenCalledWith(expect.objectContaining({ connection: expect.objectContaining({ name: valid }) })));
});

test("editing uses the same Unicode limit and preserves an invalid name for correction", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  vi.mocked(api.update).mockResolvedValue();
  render(<App />);
  fireEvent.click(await screen.findByText("Edit drive"));
  const input = screen.getByLabelText("Drive name") as HTMLInputElement;
  fireEvent.change(input, { target: { value: "é".repeat(81) } });
  fireEvent.click(screen.getByText("Save changes"));
  await screen.findByText("name must contain between 1 and 80 characters");
  expect(api.update).not.toHaveBeenCalled();
  expect(input.value).toHaveLength(81);
  fireEvent.change(input, { target: { value: ` \u0085${"é".repeat(80)} ` } });
  fireEvent.click(screen.getByText("Save changes"));
  await waitFor(() => expect(api.update).toHaveBeenCalledWith(expect.objectContaining({ name: "é".repeat(80) })));
});

test("structured action and snapshot errors translate even when English wording changes", async () => {
  const cause = { code: "recovery.required_before_exit", params: { name: "My {letter} 😀" }, message: "Changed English wording" };
  vi.mocked(api.exit).mockRejectedValue(new Error(cause.message, { cause }));
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" },
    connections: [{ ...connection, error: "Changed mount wording", errorMessage: { ...bridgeFixtures.samples.message, message: "Changed mount wording" } }],
  });
  render(<App />);
  fireEvent.mouseEnter(await screen.findByRole("button", { name: "Alertas e recuperação" }));
  await screen.findByText("letra S: já está em uso");
  fireEvent.click(screen.getByText("Sair com segurança"));
  await screen.findByText("recupere a conexão My {letter} 😀 antes de sair com segurança");
});

test("structured field errors keep their field association and translate by code", async () => {
  const cause = { ...bridgeFixtures.samples.dynamicFieldError, field: "name", message: "Changed field wording" };
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" }, connections: [connection] });
  vi.mocked(api.update).mockRejectedValue(new Error(cause.message, { cause }));
  render(<App />);
  fireEvent.click(await screen.findByText("Editar unidade"));
  fireEvent.click(screen.getByText("Salvar alterações"));
  const error = await screen.findByText("preencha Server");
  const input = screen.getByLabelText("Nome da unidade") as HTMLInputElement;
  expect(input.getAttribute("aria-describedby")).toBe(error.id);
  expect(input.value).toBe(connection.name);
  await waitFor(() => expect(document.activeElement).toBe(input));
});

test("About shows actual versions and unavailable components, and returns focus on close", async () => {
  vi.mocked(api.about).mockResolvedValue({ appVersion: "0.1.0", rcloneVersion: "v-running", windowsVersion: "10.0.26100", winFspVersion: "", webView2Version: "123.4", architecture: "amd64" });
  render(<App />);
  const button = await screen.findByRole("button", { name: "About SDrive" });
  button.focus();
  fireEvent.click(button);
  await screen.findByText("v-running");
  expect(screen.getByRole("dialog").textContent).toContain("Unavailable or not installed");
  cancelDialog();
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(document.activeElement).toBe(button);
});

test("About uses Portuguese and reports a failed version request", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" } });
  vi.mocked(api.about).mockRejectedValue(new Error("version request failed"));
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "Sobre o SDrive" }));
  await screen.findByText("version request failed");
  fireEvent.click(screen.getByRole("button", { name: "Fechar" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("diagnostics previews and copies exactly the reviewed report during settings recovery", async () => {
  const report = "SDrive diagnostic report v1\nengine_exit unexpected network exit=17\n";
  vi.mocked(api.about).mockRejectedValue(new Error("versions unavailable"));
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settingsError: "Settings damaged" });
  vi.mocked(api.diagnostics).mockResolvedValue(report);
  vi.mocked(api.copyDiagnostics).mockResolvedValue();
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "About SDrive" }));
  expect(api.diagnostics).not.toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "Copy report" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "View diagnostics" }));
  const preview = await screen.findByRole("textbox", { name: "Diagnostic report (technical text in English)" });
  expect((preview as HTMLTextAreaElement).value).toBe(report);
  expect((preview as HTMLTextAreaElement).readOnly).toBe(true);
  expect(api.copyDiagnostics).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Copy report" }));
  await screen.findByText("Report copied.");
  expect(api.copyDiagnostics).toHaveBeenCalledWith(report);
});

test("diagnostics offers retry and manual selection when loading or copying fails", async () => {
  vi.mocked(api.about).mockResolvedValue({ appVersion: "0.1.0", rcloneVersion: "", windowsVersion: "", winFspVersion: "", webView2Version: "", architecture: "amd64" });
  vi.mocked(api.diagnostics).mockRejectedValueOnce(new Error("private backend error")).mockResolvedValue("safe report");
  vi.mocked(api.copyDiagnostics).mockRejectedValue(new Error("private clipboard error"));
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "About SDrive" }));
  fireEvent.click(screen.getByRole("button", { name: "View diagnostics" }));
  await screen.findByText("Could not load the diagnostic report. Try again.");
  fireEvent.click(screen.getByRole("button", { name: "View diagnostics" }));
  await screen.findByRole("textbox");
  fireEvent.click(screen.getByRole("button", { name: "Copy report" }));
  await screen.findByText("Could not copy the report. Select and copy the text manually.");
  expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("safe report");
  expect(screen.queryByText(/private/)).toBeNull();
  cancelDialog();
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("diagnostics controls and clipboard feedback use Portuguese", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" } });
  vi.mocked(api.about).mockResolvedValue({ appVersion: "0.1.0", rcloneVersion: "", windowsVersion: "", winFspVersion: "", webView2Version: "", architecture: "amd64" });
  vi.mocked(api.diagnostics).mockResolvedValue("SDrive diagnostic report v1");
  vi.mocked(api.copyDiagnostics).mockResolvedValue();
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "Sobre o SDrive" }));
  fireEvent.click(screen.getByRole("button", { name: "Ver diagnóstico" }));
  await screen.findByRole("textbox", { name: "Relatório de diagnóstico (texto técnico em inglês)" });
  fireEvent.click(screen.getByRole("button", { name: "Copiar relatório" }));
  await screen.findByText("Relatório copiado.");
});

test("closing About ignores a late diagnostic response", async () => {
  vi.mocked(api.about).mockResolvedValue({ appVersion: "0.1.0", rcloneVersion: "", windowsVersion: "", winFspVersion: "", webView2Version: "", architecture: "amd64" });
  let resolve!: (report: string) => void;
  vi.mocked(api.diagnostics).mockImplementation(() => new Promise((done) => { resolve = done; }));
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "About SDrive" }));
  fireEvent.click(screen.getByRole("button", { name: "View diagnostics" }));
  expect((screen.getByRole("button", { name: "Loading diagnostic report…" }) as HTMLButtonElement).disabled).toBe(true);
  cancelDialog();
  await act(async () => { resolve("late report"); });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(api.copyDiagnostics).not.toHaveBeenCalled();
});

test("hidden polling stops and showing refreshes immediately without accepting a late snapshot", async () => {
  vi.useFakeTimers();
  let visibility!: (visible: boolean) => void;
  const unsubscribe = vi.fn();
  vi.mocked(api.onDesktopVisibility).mockImplementation((callback) => { visibility = callback; return unsubscribe; });
  const view = render(<App />);
  try {
    await act(async () => {});
    expect(api.snapshot).toHaveBeenCalledTimes(1);
    let resolveLate!: (value: typeof snapshot) => void;
    vi.mocked(api.snapshot).mockImplementationOnce(() => new Promise((resolve) => { resolveLate = resolve; }));
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(api.snapshot).toHaveBeenCalledTimes(2);
    act(() => visibility(false));
    await act(async () => { resolveLate({ ...snapshot, engineError: "obsolete hidden result" }); });
    expect(screen.queryByText("obsolete hidden result")).toBeNull();
    await act(async () => { await vi.advanceTimersByTimeAsync(30000); });
    expect(api.snapshot).toHaveBeenCalledTimes(2);
    await act(async () => { visibility(true); });
    expect(api.snapshot).toHaveBeenCalledTimes(3);
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(api.snapshot).toHaveBeenCalledTimes(4);
    view.unmount();
    expect(unsubscribe).toHaveBeenCalledOnce();
  } finally { vi.useRealTimers(); }
});

test("background startup suspends visual polling after the visibility handshake", async () => {
  vi.useFakeTimers();
  vi.mocked(api.desktopVisible).mockResolvedValue(false);
  const view = render(<App />);
  try {
    await act(async () => {});
    await act(async () => { await vi.advanceTimersByTimeAsync(30000); });
    expect(api.snapshot).toHaveBeenCalledTimes(1);
    view.unmount();
  } finally { vi.useRealTimers(); }
});

test("an older initial visibility response cannot override a newer native event", async () => {
  vi.useFakeTimers();
  let visibility!: (visible: boolean) => void;
  let initialVisibility!: (visible: boolean) => void;
  vi.mocked(api.onDesktopVisibility).mockImplementation((callback) => { visibility = callback; return () => {}; });
  vi.mocked(api.desktopVisible).mockReturnValue(new Promise(resolve => { initialVisibility = resolve; }));
  const view = render(<App />);
  try {
    await act(async () => {});
    await act(async () => { visibility(false); visibility(true); });
    const count = vi.mocked(api.snapshot).mock.calls.length;
    await act(async () => { initialVisibility(false); });
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(api.snapshot).toHaveBeenCalledTimes(count + 1);
    view.unmount();
  } finally { vi.useRealTimers(); }
});

test("damaged settings block normal actions and restoration requires confirmation", async () => {
  const damaged = { ...snapshot, settingsError: "Could not load local settings. Preserve the data directory and restore a valid backup.", backupAvailable: true };
  vi.mocked(api.snapshot).mockResolvedValue(damaged);
  render(<App />);
  await screen.findByRole("heading", { name: "Settings recovery" });
  expect((screen.getByText("+ New connection") as HTMLButtonElement).disabled).toBe(true);
  expect(screen.queryByText("Add my first connection")).toBeNull();
  fireEvent.click(screen.getByText("Restore saved backup"));
  expect(screen.getByRole("dialog").textContent).toContain("may omit newer connections");
  expect(api.restoreSettings).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(api.restoreSettings).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText("Restore saved backup"));
  vi.mocked(api.restoreSettings).mockResolvedValue();
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, recoveryHold: true });
  fireEvent.click(screen.getByText("Confirm restoration"));
  await screen.findByText("Settings recovery is protected");
  expect(api.restoreSettings).toHaveBeenCalledTimes(1);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(api.mount).not.toHaveBeenCalled();
  expect(api.begin).not.toHaveBeenCalled();
});

test("unavailable backup leaves settings protected and allows closing", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settingsError: "Settings are unreadable" });
  render(<App />);
  await screen.findByRole("heading", { name: "Settings recovery" });
  expect(screen.queryByText("Restore saved backup")).toBeNull();
  expect(screen.getByText("No compatible backup is available. Preserve the data directory for manual recovery.")).toBeDefined();
  fireEvent.click(screen.getByText("Close SDrive"));
  await waitFor(() => expect(api.exit).toHaveBeenCalledTimes(1));
});

test("failed restoration stays in the confirmation dialog with an actionable error", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settingsError: "Settings are unreadable", backupAvailable: true });
  vi.mocked(api.restoreSettings).mockRejectedValue(new Error("could not archive damaged settings; restoration cancelled"));
  render(<App />);
  fireEvent.click(await screen.findByText("Restore saved backup"));
  fireEvent.click(screen.getByText("Confirm restoration"));
  await screen.findByText("could not archive damaged settings; restoration cancelled");
  expect(screen.getByRole("dialog").textContent).toContain("restoration cancelled");
  expect(api.mount).not.toHaveBeenCalled();
});

test("cleanup warnings can be retried without reporting an engine failure", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, maintenanceError: "Credential cleanup could not finish. Saved connections remain available; try cleanup again.", connections: [connection] });
  render(<App />);
  await screen.findByText("My Drive");
  expect(screen.queryByText("Restart engine")).toBeNull();
  vi.mocked(api.retryCleanup).mockResolvedValue();
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  fireEvent.click(screen.getByText("Retry cleanup"));
  await screen.findByText("Credential cleanup completed.");
  expect(api.retryCleanup).toHaveBeenCalledTimes(1);
});

test("unresolved recovery remains visible after verification fails", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, recoveryHold: true });
  vi.mocked(api.verifyRecovery).mockRejectedValue(new Error("credentials absent from restored settings remain preserved; manual recovery is required"));
  render(<App />);
  fireEvent.click(await screen.findByText("Verify recovery"));
  await screen.findByText("credentials absent from restored settings remain preserved; manual recovery is required");
  expect(screen.getByText("Settings recovery is protected")).toBeDefined();
  expect(api.begin).not.toHaveBeenCalled();
});

test("recovery and startup warnings use the saved interface language", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" }, recoveryHold: true, startupWarning: "Windows startup may differ from saved preferences. Save preferences to reconcile it." });
  render(<App />);
  await screen.findByText("A recuperação das configurações está protegida");
  expect(screen.getByText("Verificar recuperação")).toBeDefined();
  expect(screen.getByText("A inicialização do Windows pode diferir das preferências salvas. Salve as preferências para sincronizá-la.")).toBeDefined();
});

test("dialog opens modally, handles cancellation and restores the opener", async () => {
  const show = vi.spyOn(HTMLDialogElement.prototype, "showModal");
  render(<React.StrictMode><App /></React.StrictMode>);
  const opener = await screen.findByText("Add my first connection");
  opener.focus();
  fireEvent.click(opener);
  selectAvailableCategory();
  expect(show).toHaveBeenCalledTimes(2);
  expect(screen.getByRole("dialog").hasAttribute("open")).toBe(true);
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Close setup" }));
  cancelDialog();
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(document.activeElement).toBe(opener);
  show.mockRestore();
});

test("empty state opens accessible connection form", async () => {
  render(<App />);
  await screen.findByText("One letter. All your files.");
  fireEvent.click(screen.getByText("Add my first connection"));
  selectAvailableCategory();
  expect(screen.getByRole("dialog").getAttribute("aria-modal")).toBe("true");
  expect(screen.getByLabelText("Drive name")).toBeDefined();
});
test("cancel OAuth cleans up draft", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({
    authenticated: false, connectionID: "",
    id: "draft",
    busy: true,
    complete: false,
    question: null,
    error: "",
  });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.change(screen.getByLabelText("Drive name"), {
    target: { value: "Conta" },
  });
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  await screen.findByText("Waiting for authentication…");
  fireEvent.click(screen.getByText("Cancel setup"));
  await waitFor(() => expect(api.end).toHaveBeenCalledWith("draft"));
});
test("recovery prevents deleting or changing destination", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    connections: [{ ...connection, recovery: true }],
  });
  render(<App />);
  await screen.findByText("Recovery required");
  expect((screen.getByText("Edit drive") as HTMLButtonElement).disabled).toBe(
    true,
  );
  expect(
    (screen.getByText("Remove connection") as HTMLButtonElement).disabled,
  ).toBe(true);
});
test("safe-exit errors remain visible", async () => {
  vi.mocked(api.exit).mockRejectedValue(new Error("Uploads pendentes"));
  render(<App />);
  fireEvent.click(await screen.findByText("Exit safely"));
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    expect.stringContaining("Uploads pendentes"),
  );
});
test("provider question accepts MFA and does not persist it in browser storage", async () => {
  vi.mocked(api.begin).mockResolvedValue("mfa");
  vi.mocked(api.status).mockResolvedValue({
    authenticated: false, connectionID: "",
    id: "mfa",
    busy: false,
    complete: false,
    error: "",
    question: { Unsupported: false,
      Name: "MFA code",
      Help: "Confirme o segundo fator",
      DefaultStr: "",
      Type: "string",
      IsPassword: true,
      Sensitive: true,
      Required: true,
      Exclusive: false,
      Examples: null,
    },
  });
  vi.mocked(api.answer).mockResolvedValue();
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.change(screen.getByLabelText("Drive name"), {
    target: { value: "Conta" },
  });
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  const code = await screen.findByLabelText("MFA code");
  expect(code.getAttribute("type")).toBe("password");
  expect(document.getElementById(code.getAttribute("aria-describedby")!)?.textContent).toBe("Confirme o segundo fator");
  fireEvent.change(code, { target: { value: "123456" } });
  fireEvent.click(screen.getByRole("button", { name: "Show MFA code" }));
  expect(code.getAttribute("type")).toBe("text");
  expect(screen.getByRole("button", { name: "Hide MFA code" }).getAttribute("aria-pressed")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Hide MFA code" }));
  expect(code.getAttribute("type")).toBe("password");
  fireEvent.click(screen.getByText("Continue"));
  await waitFor(() => expect(api.answer).toHaveBeenCalledWith("mfa", "123456"));
  expect(localStorage.length).toBe(0);
  expect(sessionStorage.length).toBe(0);
});

const ftpProvider = { oauth: false,
  id: "ftp", name: "FTP / FTPS", backend: "ftp", transportEncryption: "optional", experimental: false, available: true,
  fields: [
    { advanced: false, file: false, key: "host", label: "Server", secret: false, required: true, default: "" },
    { advanced: false, file: false, key: "user", label: "Username", secret: false, required: true, default: "" },
    { advanced: false, file: false, key: "pass", label: "Password", secret: true, required: false, default: "" },
  ],
};

test.each(["en", "pt-BR"])("drive labels and experimental text follow %s without translating account content", async (language) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot,
    settings: { ...snapshot.settings, language: language as "en" | "pt-BR" },
    providers: [{ ...snapshot.providers[0], experimental: true }],
    connections: [{ ...connection, name: "Root", path: "My drives" }],
  });
  render(<App />);
  await screen.findByRole("region", { name: language === "en" ? "Drives" : "Unidades" });
  expect(await screen.findByRole("heading", { name: "Root" })).toBeDefined();
  expect(screen.getByText("Google Drive · Experimental")).toBeDefined();
  fireEvent.mouseEnter(screen.getByRole("button", { name: language === "en" ? "Remote destination" : "Destino remoto" }));
  expect(screen.getByRole("tooltip").textContent).toContain("Google Drive · My drives");
  fireEvent.click(screen.getByRole("button", { name: language === "en" ? "+ New connection" : "+ Nova conexão" }));
  selectAvailableCategory();
  expect(screen.getByRole("option", { name: "Google Drive (experimental)" })).toBeDefined();
});

test("drive announcements ignore initial state, unchanged polling, counters and language changes", async () => {
  vi.useFakeTimers();
  let current: Snapshot = { ...snapshot, connections: [connection] };
  vi.mocked(api.snapshot).mockImplementation(async () => current);
  const view = render(<App />);
  try {
    await act(async () => {});
    const live = view.container.querySelector('[aria-live="polite"][aria-atomic="true"]')!;
    expect(live.textContent).toBe("");
    const mutations: MutationRecord[] = [];
    const observer = new MutationObserver((records) => mutations.push(...records));
    observer.observe(live, { subtree: true, characterData: true, childList: true });
    const poll = async () => { await act(async () => { await vi.advanceTimersByTimeAsync(3000); }); };
    current = { ...current, connections: [{ ...connection, mounted: true, recovery: true }] };
    await poll();
    expect(live.textContent).toBe("My Drive: Connected");
    mutations.length = 0;
    current = { ...current, connections: [{ ...connection, mounted: true, recovery: true, pending: 5 }] };
    await poll();
    current = { ...current, settings: { ...current.settings, language: "pt-BR" } };
    await poll();
    expect(mutations).toHaveLength(0);
    current = { ...current, connections: [{ ...connection, recovery: true }] };
    await poll();
    expect(live.textContent).toBe("My Drive: Recuperação necessária");
    current = { ...current, connections: [{ ...connection, pending: null }] };
    await poll();
    expect(live.textContent).toBe("My Drive: Envios pendentes desconhecidos");
    current = { ...current, connections: [connection] };
    await poll();
    expect(live.textContent).toBe("My Drive: Desconectada");
    observer.disconnect();
  } finally {
    view.unmount();
    vi.useRealTimers();
  }
});

test("drive action disclosure closes on Escape, outside interaction, focus departure and actions", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  render(<App />);
  const summary = await screen.findByLabelText("More actions for My Drive");
  const details = summary.closest("details")!;
  fireEvent.click(summary);
  expect(details.open).toBe(true);
  screen.getByRole("button", { name: "Test access" }).focus();
  fireEvent.keyDown(document.activeElement!, { key: "Escape" });
  expect(details.open).toBe(false);
  expect(document.activeElement).toBe(summary);
  fireEvent.click(summary);
  const outside = screen.getByRole("button", { name: "+ New connection" });
  fireEvent(outside, new Event("pointerdown", { bubbles: true }));
  expect(details.open).toBe(false);
  outside.focus();
  expect(document.activeElement).toBe(outside);
  fireEvent.click(summary);
  summary.focus();
  outside.focus();
  expect(details.open).toBe(false);
  fireEvent.click(summary);
  fireEvent.click(screen.getByRole("button", { name: "Test access" }));
  expect(api.test).toHaveBeenCalledWith(connection.id);
  expect(details.open).toBe(false);
  expect(document.activeElement).toBe(summary);
  await act(async () => {});
});

test("field help survives validation and secret reveal, and security consequences are described", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [{ ...ftpProvider,
    fields: ftpProvider.fields.map((f) => ({ ...f, hint: `${f.label} instructions` })),
  }], connections: [{ ...connection, provider: "ftp" }] });
  vi.mocked(api.fields).mockResolvedValue({ values: { host: "example.test", user: "original" }, transport: "explicit" });
  vi.mocked(api.begin).mockRejectedValue(new Error("invalid password", { cause: { field: "pass", message: "invalid password" } }));
  render(<App />);
  fireEvent.click(await screen.findByText("Reconfigure account"));
  await waitFor(() => expect((screen.getByLabelText("Username") as HTMLInputElement).value).toBe("original"));
  const password = screen.getByLabelText("Password");
  const help = screen.getByText("Password instructions");
  expect(password.getAttribute("aria-describedby")).toBe(help.id);
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  const error = await screen.findByText("invalid password");
  expect(password.getAttribute("aria-describedby")?.split(" ")).toEqual([help.id, error.id]);
  fireEvent.click(screen.getByRole("button", { name: "Show Password" }));
  expect(password.getAttribute("aria-describedby")?.split(" ")).toEqual([help.id, error.id]);
  fireEvent.change(password, { target: { value: "synthetic" } });
  expect(password.getAttribute("aria-describedby")).toBe(help.id);
  for (const label of ["Access type", "Allow unencrypted connection (HTTP / FTP)", "Mount read only", "Connect automatically when SDrive starts"]) {
    const control = screen.getByLabelText(label);
    const description = document.getElementById(control.getAttribute("aria-describedby")!);
    expect(description?.textContent).toBeTruthy();
  }
});

test("native dialog cancellation keeps a busy preference save open", async () => {
  let finish!: () => void;
  vi.mocked(api.settings).mockReturnValue(new Promise((resolve) => { finish = resolve; }));
  render(<App />);
  await waitFor(() => expect((screen.getByRole("button", { name: /Preferences/ }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(screen.getByRole("button", { name: /Preferences/ }));
  const cache = screen.getByLabelText("Cache target per connection (GiB)");
  expect(document.getElementById(cache.getAttribute("aria-describedby")!)?.textContent).toContain("Applied on the next mount");
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  cancelDialog();
  expect(screen.getByRole("dialog").hasAttribute("open")).toBe(true);
  await act(async () => finish());
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("native cancellation preserves setup when asynchronous cleanup fails and allows retry", async () => {
  vi.mocked(api.begin).mockResolvedValue("cancel-fixture");
  vi.mocked(api.status).mockResolvedValue({ authenticated: false, connectionID: "", id: "cancel-fixture", busy: true, complete: false, question: null, error: "" });
  vi.mocked(api.end).mockRejectedValueOnce(new Error("cleanup failed"));
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  fireEvent.click(screen.getByRole("button", { name: "Sign in with Google Drive" }));
  await screen.findByText("Waiting for authentication…");
  cancelDialog();
  await screen.findByText("cleanup failed");
  expect(screen.getByRole("dialog").hasAttribute("open")).toBe(true);
  vi.mocked(api.end).mockResolvedValue();
  cancelDialog();
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(api.end).toHaveBeenCalledTimes(2);
});

test.each(["explicit", "implicit", "plain"])("FTP renewal restores %s and requires fresh plain FTP consent", async (transport) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [...snapshot.providers, ftpProvider], connections: [{ ...connection, provider: "ftp", recovery: true }] });
  vi.mocked(api.fields).mockResolvedValue({ values: { host: "example.test", user: "original" }, transport });
  vi.mocked(api.begin).mockResolvedValue("renew");
  vi.mocked(api.status).mockResolvedValue({ authenticated: false, connectionID: "", id: "renew", busy: true, complete: false, question: null, error: "" });
  render(<App />);
  fireEvent.click(await screen.findByText("Reconfigure account"));
  await waitFor(() => expect((screen.getByLabelText("Server") as HTMLInputElement).value).toBe("example.test"));
  expect((screen.getByLabelText("Access type") as HTMLSelectElement).value).toBe(transport);
  const consent = screen.getByLabelText("Allow unencrypted connection (HTTP / FTP)") as HTMLInputElement;
  expect(consent.checked).toBe(false);
  fireEvent.change(screen.getByLabelText("Password"), { target: { value: "synthetic-secret" } });
  if (transport === "plain") {
    expect(screen.getByText("This connection uses unencrypted FTP. Confirm permission in Advanced options before continuing.")).toBeDefined();
    fireEvent.click(consent);
  }
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await waitFor(() => expect(api.begin).toHaveBeenCalledWith(expect.objectContaining({ transport, allowInsecure: transport === "plain", values: { host: "example.test", user: "original", pass: "synthetic-secret" } })));
});

test("failed credential loading prevents renewal with default transport", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [...snapshot.providers, ftpProvider], connections: [{ ...connection, provider: "ftp" }] });
  vi.mocked(api.fields).mockRejectedValue(new Error("Could not load access details. Cancel and try again."));
  render(<App />);
  fireEvent.click(await screen.findByText("Reconfigure account"));
  await screen.findByText("Could not load access details. Cancel and try again.");
  expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(true);
  expect(api.begin).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
});

test("password visibility is local and resets when access is reopened", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [...snapshot.providers, ftpProvider] });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.change(screen.getByLabelText("Connection type"), { target: { value: "ftp" } });
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  const password = screen.getByLabelText("Password");
  fireEvent.change(password, { target: { value: "synthetic-secret" } });
  expect(password.getAttribute("type")).toBe("password");
  const toggle = screen.getByRole("button", { name: "Show Password" });
  toggle.focus();
  expect(document.activeElement).toBe(toggle);
  fireEvent.click(toggle);
  expect(password.getAttribute("type")).toBe("text");
  expect(api.begin).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Back" }));
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  expect(screen.getByLabelText("Password").getAttribute("type")).toBe("password");
  expect((screen.getByLabelText("Password") as HTMLInputElement).value).toBe("synthetic-secret");
  expect(localStorage.length).toBe(0);
  expect(sessionStorage.length).toBe(0);
});

test("existing user-owned OAuth connection can renew with its saved identity", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: snapshot.providers, connections: [connection] });
  vi.mocked(api.fields).mockResolvedValue({ values: { client_id: "custom-public-id" }, transport: "" });
  vi.mocked(api.begin).mockResolvedValue("custom");
  vi.mocked(api.status).mockResolvedValue({ authenticated: false, connectionID: "", id: "custom", busy: true, complete: false, question: null, error: "" });
  render(<App />);
  fireEvent.click(await screen.findByText("Reconfigure account"));
  await waitFor(() => expect((screen.getByRole("button", { name: "Sign in with Google Drive" }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(screen.getByRole("button", { name: "Sign in with Google Drive" }));
  await waitFor(() => expect(api.begin).toHaveBeenCalledWith(expect.objectContaining({ values: { client_id: "custom-public-id" } })));
});

test("password controls and FTP warning follow the saved Portuguese language", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" }, providers: [...snapshot.providers, ftpProvider], connections: [{ ...connection, provider: "ftp" }] });
  vi.mocked(api.fields).mockResolvedValue({ values: { host: "example.test", user: "original" }, transport: "plain" });
  render(<App />);
  fireEvent.click(await screen.findByText("Reconfigurar conta"));
  await screen.findByText("Esta conexão usa FTP sem criptografia. Confirme a autorização nas opções avançadas antes de continuar.");
  fireEvent.click(screen.getByRole("button", { name: "Mostrar Senha" }));
  expect(screen.getByLabelText("Senha").getAttribute("type")).toBe("text");
  expect(screen.getByRole("button", { name: "Ocultar Senha" })).toBeDefined();
});

test("guided setup chooses a folder before confirming and mounting", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({connectionID: "",
    id: "draft",
    busy: false,
    complete: false,
    authenticated: true,
    question: null,
    error: "",
  });
  vi.mocked(api.folders).mockResolvedValue([
    { name: "Documents", path: "Documents" },
  ]);
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  fireEvent.click(await screen.findByRole("button", { name: "▱ Documents" }));
  await waitFor(() =>
    expect(api.folders).toHaveBeenCalledWith("draft", "Documents"),
  );
  fireEvent.click(screen.getByText("Connect drive"));
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledWith("draft", "Documents", { letter: "S", folder: "", mode: "" });
  expect(api.mount).toHaveBeenCalledWith("connected-id");
  expect(api.begin).toHaveBeenCalledWith(
    expect.objectContaining({
      guided: true,
      connection: expect.objectContaining({ name: "Google Drive" }),
    }),
  );
});

test("destination validation errors prevent mounting", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({connectionID: "",
    id: "draft",
    busy: false,
    complete: false,
    authenticated: true,
    question: null,
    error: "",
  });
  vi.mocked(api.confirm).mockRejectedValue(
    new Error("Check account permissions"),
  );
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  const connect = await screen.findByText("Connect drive");
  await waitFor(() =>
    expect((connect as HTMLButtonElement).disabled).toBe(false),
  );
  fireEvent.click(connect);
  await screen.findByText("Check account permissions");
  expect(api.mount).not.toHaveBeenCalled();
});

test("OAuth setup requires a user-owned application and shows mandatory fields directly", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [{ ...snapshot.providers[0], fields: [
    { key: "client_id", label: "Client ID", secret: false, required: false, default: "", advanced: false, file: false },
    { key: "client_secret", label: "Client Secret", secret: true, required: false, default: "", advanced: false, file: false },
  ] }] });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  expect(screen.getByRole("tab", { name: "Basic" }).getAttribute("aria-selected")).toBe("true");
  fireEvent.click(screen.getByRole("tab", { name: "Advanced" }));
  expect(screen.getByText(/These connections require additional setup/)).toBeDefined();
  fireEvent.click(screen.getByText("Continue"));
  expect((screen.getByLabelText("Client ID") as HTMLInputElement).required).toBe(true);
  expect((screen.getByLabelText("Client Secret") as HTMLInputElement).required).toBe(true);
  expect(screen.getByLabelText("Client ID").closest("details")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Sign in with Google Drive" }));
  expect(api.begin).not.toHaveBeenCalled();
});

test("cancelling after authentication keeps the draft unconfirmed", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({connectionID: "",
    id: "draft",
    busy: false,
    complete: false,
    authenticated: true,
    question: null,
    error: "",
  });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  await screen.findByText("Choose what to connect");
  fireEvent.click(screen.getByText("Cancel"));
  await waitFor(() => expect(api.end).toHaveBeenCalledWith("draft"));
  expect(api.confirm).not.toHaveBeenCalled();
});

test("GCS service-account access is in Basic without OAuth fields", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    providers: [
      ...snapshot.providers,
      {
        id: "gcs",
        name: "Google Cloud Storage",
        backend: "google cloud storage", transportEncryption: "required",
        experimental: false,
        oauth: true,
        available: true,
        fields: [
          { advanced: false,
            key: "service_account_file",
            label: "JSON file",
            secret: false,
            required: false,
            default: "",
            file: true,
          },
        ],
      },
    ],
  });
  vi.mocked(api.selectFile).mockResolvedValue("C:\\Credentials\\account.json");
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.change(screen.getByLabelText("Connection type"), {
    target: { value: "gcs:serviceAccount" },
  });
  expect(screen.getByRole("tab", { name: "Basic" }).getAttribute("aria-selected")).toBe("true");
  expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(false);
  fireEvent.click(screen.getByText("Continue"));
  expect(screen.getByText("Select a service account JSON file authorized for your bucket. Browser sign-in is not required.")).toBeDefined();
  fireEvent.click(screen.getByText("Select file…"));
  await waitFor(() =>
    expect((screen.getByLabelText("JSON file") as HTMLInputElement).value).toBe(
      "C:\\Credentials\\account.json",
    ),
  );
  expect(api.selectFile).toHaveBeenCalledWith("service_account_file");
  expect(
    (
      screen.getByRole("button", {
        name: "Continue",
      }) as HTMLButtonElement
    ).disabled,
  ).toBe(false);
});

test("mount failure keeps the saved connection and retries without duplicating it", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({connectionID: "",
    id: "draft",
    busy: false,
    complete: false,
    authenticated: true,
    question: null,
    error: "",
  });
  vi.mocked(api.mount).mockRejectedValueOnce(new Error("Letter in use"));
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  const connect = await screen.findByText("Connect drive");
  await waitFor(() =>
    expect((connect as HTMLButtonElement).disabled).toBe(false),
  );
  fireEvent.click(connect);
  await screen.findByText("Letter in use");
  fireEvent.click(connect);
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledTimes(1);
  expect(api.mount).toHaveBeenCalledTimes(2);
  expect(api.mount).toHaveBeenNthCalledWith(1, "connected-id");
  expect(api.mount).toHaveBeenNthCalledWith(2, "connected-id");
});

test("destination validation failure does not mount and allows retry", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({connectionID: "",
    id: "draft",
    busy: false,
    complete: false,
    authenticated: true,
    question: null,
    error: "",
  });
  vi.mocked(api.confirm).mockRejectedValueOnce(new Error("Folder unavailable"));
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  const connect = await screen.findByRole("button", { name: "Connect drive" });
  await waitFor(() => expect((connect as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(connect);
  await screen.findByText("Folder unavailable");
  expect(api.mount).not.toHaveBeenCalled();
  expect(screen.queryByText("Drive connected")).toBeNull();
  fireEvent.click(connect);
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledTimes(2);
  expect(api.confirm).toHaveBeenLastCalledWith("draft", "", { letter: "S", folder: "", mode: "" });
  expect(api.mount).toHaveBeenCalledExactlyOnceWith("connected-id");
});

test("unknown uploads never display Connected or a complete zero total", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    connections: [{ ...connection, mounted: true, pending: null,
      uploads: { active: 1, queued: 2, cacheErrors: 0 },
      cache: { checkedAt: null, usedBytes: null, pendingBytes: null, error: "Could not inspect local cache; preserve the files and try again." },
    }, { ...connection, id: "other", name: "Other drive", pending: 3 }],
  });
  render(<App />);
  await screen.findByText("Pending uploads unknown");
  expect(screen.queryByText("Connected", { exact: true })).toBeNull();
  expect(screen.getByText("3 known; others unknown")).toBeDefined();
  fireEvent.mouseEnter(screen.getAllByRole("button", { name: "Upload details" })[0]);
  expect(screen.getByText("1 uploading · 2 queued · 0 cache errors")).toBeDefined();
  fireEvent.mouseEnter(screen.getAllByRole("button", { name: "Local cache" })[0]);
  expect(screen.getByText("Approximate cache used: — · Pending file sizes: —")).toBeDefined();
  expect(screen.getByRole("button", { name: "Disconnect" })).toBeDefined();
});

test.each(["en", "pt-BR"] as const)("mounted cache protection is not a recovery alert in %s", async (language) => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot, settings: { ...snapshot.settings, language },
    connections: [{ ...connection, mounted: true, recovery: true,
      transportSecurity: { state: "encrypted", protocol: "HTTPS", source: "", checkedAt: null },
    }],
  });
  render(<App />);
  const alerts = await screen.findByRole("button", { name: language === "en" ? "Alerts and recovery" : "Alertas e recuperação" });
  expect(alerts.classList.contains("attention")).toBe(false);
  expect(screen.queryByText(language === "en" ? "Recovery required" : "Recuperação necessária")).toBeNull();
  fireEvent.mouseEnter(alerts);
  expect(screen.getByText(language === "en" ? "No alerts or recovery required." : "Nenhum alerta ou recuperação necessária.")).toBeDefined();
  expect(screen.queryByText(/Preserved destination:|Destino preservado:/)).toBeNull();
});

test("mounted cache protection keeps actual upload errors visible", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot, connections: [{ ...connection, mounted: true, recovery: true, pending: null,
      uploads: { active: 0, queued: 0, cacheErrors: 1 },
      transportSecurity: { state: "encrypted", protocol: "HTTPS", source: "", checkedAt: null },
    }],
  });
  render(<App />);
  const alerts = await screen.findByRole("button", { name: "Alerts and recovery" });
  expect(alerts.classList.contains("attention")).toBe(true);
  fireEvent.mouseEnter(alerts);
  expect(screen.getByRole("tooltip").textContent).toContain("Pending uploads unknown");
  expect(screen.getByText("0 uploading · 0 queued · 1 cache errors")).toBeDefined();
  expect(screen.queryByText(/Recover using the original account and destination/)).toBeNull();
});

test("recovery explains the preserved destination and measured cache", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot, freeBytes: 10 * 1024 ** 3,
    connections: [{ ...connection, recovery: true, path: "Original library", pending: 2,
      cache: { checkedAt: "2026-10-23T12:00:00Z", usedBytes: 1024, pendingBytes: 2048, error: "" },
    }],
  });
  render(<App />);
  fireEvent.mouseEnter(await screen.findByRole("button", { name: "Alerts and recovery" }));
  await screen.findByText("Preserved destination: Google Drive · Original library", { exact: false });
  expect(screen.getByText(/Recover using the original account and destination/)).toBeDefined();
  fireEvent.mouseEnter(screen.getByRole("button", { name: "Local cache" }));
  expect(screen.getByText("Approximate cache used: 1.0 KiB · Pending file sizes: 2.0 KiB")).toBeDefined();
  expect(screen.getByText(/Free on cache disk: 10.0 GiB/)).toBeDefined();
  expect(screen.getByText(/updated every 30 s/)).toBeDefined();
  expect(screen.getByRole("button", { name: "Recover" })).toBeDefined();
});

test("unknown pending status and recovery messages are translated", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" },
    connections: [{ ...connection, recovery: true, pending: null,
      cache: { ...connection.cache, error: "Could not inspect local cache; preserve the files and try again." },
    }],
  });
  render(<App />);
  await screen.findByText("Envios pendentes desconhecidos");
  expect(screen.getByText("0 conhecidos; demais desconhecidos")).toBeDefined();
  fireEvent.mouseEnter(screen.getByRole("button", { name: "Alertas e recuperação" }));
  expect(screen.getByText(/Recupere usando a conta e o destino originais/)).toBeDefined();
  expect(screen.getByText("Não foi possível inspecionar o cache local; preserve os arquivos e tente novamente.")).toBeDefined();
});

test("failed disconnect keeps the drive visible and allows retry", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    connections: [{ ...connection, mounted: true, recovery: true }],
  });
  vi.mocked(api.unmount)
    .mockRejectedValueOnce(new Error("Pending uploads"))
    .mockResolvedValueOnce();
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "Disconnect" }));
  expect((await screen.findByRole("alert")).textContent).toContain("Pending uploads");
  expect(screen.getByRole("heading", { name: "My Drive" })).toBeDefined();
  const retry = screen.getByRole("button", { name: "Disconnect" });
  await waitFor(() => expect((retry as HTMLButtonElement).disabled).toBe(false));
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    connections: [connection],
  });
  fireEvent.click(retry);
  await screen.findByRole("button", { name: "Connect" });
  expect(api.unmount).toHaveBeenCalledTimes(2);
  expect(api.unmount).toHaveBeenLastCalledWith(connection.id);
  expect(screen.queryByRole("alert")).toBeNull();
});

test("language changes are saved, applied and restored on reopening", async () => {
  let saved = { ...snapshot.settings, language: "en" as "en" | "pt-BR" };
  vi.mocked(api.snapshot).mockImplementation(async () => ({
    ...snapshot,
    settings: saved,
  }));
  vi.mocked(api.settings).mockImplementation(async (settings) => {
    saved = { ...settings, language: settings.language || "en" };
  });
  const view = render(<App />);
  await waitFor(() => expect((screen.getByRole("button", { name: /Preferences/ }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(screen.getByRole("button", { name: /Preferences/ }));
  fireEvent.change(screen.getByLabelText("Language"), {
    target: { value: "pt-BR" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByRole("heading", { name: "Minhas unidades" });
  expect(api.settings).toHaveBeenCalledWith({
    autoStart: false,
    cacheGiB: 2,
    language: "pt-BR",
    openExplorerAfterConnect: false,
    showServiceIcons: false,
  });
  await waitFor(() => expect(document.documentElement.lang).toBe("pt-BR"));
  view.unmount();
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: /Preferências/ }));
  expect((screen.getByLabelText("Idioma") as HTMLSelectElement).value).toBe(
    "pt-BR",
  );
  fireEvent.change(screen.getByLabelText("Idioma"), {
    target: { value: "en" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Salvar" }));
  await screen.findByRole("heading", { name: "My drives" });
  await waitFor(() => expect(document.documentElement.lang).toBe("en"));
});

test("Portuguese drive UI keeps user labels and CSS state unchanged", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    settings: { ...snapshot.settings, language: "pt-BR" },
    connections: [{ ...connection, name: "Root", mounted: true }],
  });
  const view = render(<App />);
  await screen.findByRole("heading", { name: "Minhas unidades" });
  expect(screen.getByRole("heading", { name: "Root" })).toBeDefined();
  expect(view.container.querySelector(".letter.connected")).not.toBeNull();
  expect(screen.getByRole("button", { name: "Desconectar" })).toBeDefined();
  fireEvent.click(screen.getByRole("button", { name: /Preferências/ }));
  fireEvent.click(screen.getByRole("button", { name: "Cancelar" }));
  expect(api.settings).not.toHaveBeenCalled();
});

test("Portuguese wizard translates provider metadata and keeps library names", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    settings: { ...snapshot.settings, language: "pt-BR" },
    providers: [
      {
        ...snapshot.providers[0],
        fields: [
          { advanced: false, file: false,
            key: "user",
            label: "Username",
            secret: false,
            required: false,
            default: "",
          },
        ],
      },
    ],
  });
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({
    authenticated: false, connectionID: "",
    id: "draft",
    busy: false,
    complete: false,
    error: "",
    question: { Unsupported: false,
      Name: "Library",
      Help: "Select the drive or library to connect.",
      DefaultStr: "",
      Type: "string",
      IsPassword: false,
      Sensitive: false,
      Required: true,
      Exclusive: true,
      Examples: [{ Value: "library-id", Help: "Root" }],
    },
  });
  render(<App />);
  fireEvent.click(await screen.findByText("Adicionar minha primeira conexão"));
  selectAvailableCategory();
  fireEvent.click(screen.getByText("Continuar"));
  expect(screen.getByLabelText("Usuário")).toBeDefined();
  fireEvent.click(screen.getByText("Entrar com Google Drive"));
  await screen.findByLabelText("Biblioteca");
  expect(screen.getByRole("option", { name: "Root" })).toBeDefined();
  expect(screen.queryByRole("option", { name: "Raiz" })).toBeNull();
  fireEvent.change(screen.getByLabelText("Biblioteca"), {
    target: { value: "library-id" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Continuar" }));
  await waitFor(() =>
    expect(api.answer).toHaveBeenCalledWith("draft", "library-id"),
  );
});


test.each([false, true])("obsolete snapshot data and errors are ignored (reject=%s)", async (rejectOld) => {
  let resolveOld!: (value: typeof snapshot) => void;
  let reject!: (reason: Error) => void;
  const old = new Promise<typeof snapshot>((resolve, fail) => { resolveOld = resolve; reject = fail; });
  vi.mocked(api.snapshot).mockReturnValueOnce(old).mockResolvedValue({ ...snapshot, maintenanceError: "Current maintenance warning" });
  render(<App />);
  fireEvent.click(await screen.findByRole("button", { name: "Exit safely" }));
  await screen.findByText("Current maintenance warning");
  await act(async () => {
    if (rejectOld) reject(new Error("Obsolete failure"));
    else resolveOld({ ...snapshot, maintenanceError: "Obsolete maintenance warning" });
  });
  expect(screen.queryByText("Obsolete failure")).toBeNull();
  expect(screen.queryByText("Obsolete maintenance warning")).toBeNull();
  expect(screen.getByText("Current maintenance warning")).toBeDefined();
});


test("an action completing after unmount cannot start another snapshot", async () => {
  let finish!: () => void;
  vi.mocked(api.exit).mockReturnValue(new Promise<void>((resolve) => { finish = resolve; }));
  const view = render(<App />);
  await screen.findByRole("button", { name: "+ New connection" });
  fireEvent.click(screen.getByRole("button", { name: "Exit safely" }));
  view.unmount();
  await act(async () => finish());
  expect(api.snapshot).toHaveBeenCalledTimes(1);
});

test.each(["en", "pt-BR"])("no letters still allows folder-mount creation (%s)", async (language) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, letters: [], settings: { ...snapshot.settings, language: language as "en" | "pt-BR" } });
  render(<App />);
  const english = language === "en";
  const create = await screen.findByRole("button", { name: english ? "+ New connection" : "+ Nova conexão" });
  await waitFor(() => expect((create as HTMLButtonElement).disabled).toBe(false));
  expect((screen.getByRole("button", { name: english ? "Add my first connection" : "Adicionar minha primeira conexão" }) as HTMLButtonElement).disabled).toBe(false);
  fireEvent.click(create);
  expect(screen.getByRole("dialog")).toBeDefined();
  expect(api.begin).not.toHaveBeenCalled();
});

test("letters becoming unavailable blocks authentication in an open wizard", async () => {
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, letters: [] });
  await waitFor(() => expect((screen.getByRole("button", { name: "Sign in with Google Drive" }) as HTMLButtonElement).disabled).toBe(true), { timeout: 4000 });
  expect(api.begin).not.toHaveBeenCalled();
});

async function openAuthenticatedFolder() {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({connectionID: "", id: "draft", busy: false, complete: false, authenticated: true, question: null, error: "" });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  const connect = await screen.findByRole("button", { name: "Connect drive" });
  await waitFor(() => expect((connect as HTMLButtonElement).disabled).toBe(false));
  return connect;
}

test("folder mount confirmation keeps the selected local destination", async () => {
  const connect = await openAuthenticatedFolder();
  fireEvent.change(screen.getByLabelText("Mount into"), { target: { value: "folder" } });
  fireEvent.change(screen.getByLabelText("Local mount folder"), { target: { value: "C:\\Synthetic\\NewMount" } });
  expect((screen.getByLabelText("Drive presentation") as HTMLSelectElement).value).toBe("local");
  expect((screen.getByLabelText("Drive presentation") as HTMLSelectElement).disabled).toBe(true);
  fireEvent.click(connect);
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("draft", "", { letter: "", folder: "C:\\Synthetic\\NewMount", mode: "local" });
  expect(screen.getByText("Your files are available at C:\\Synthetic\\NewMount.")).toBeDefined();
});

test("cache timing preserves an explicit zero upload delay", async () => {
  vi.mocked(api.settings).mockResolvedValue();
  render(<App />);
  await waitFor(() => expect((screen.getByRole("button", { name: /Preferences/ }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(screen.getByRole("button", { name: /Preferences/ }));
  fireEvent.change(screen.getByLabelText("Cache retention after last access (hours)"), { target: { value: "48" } });
  fireEvent.change(screen.getByLabelText("Upload delay after last use (seconds)"), { target: { value: "0" } });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(api.settings).toHaveBeenCalledWith(expect.objectContaining({ cacheMaxAgeHours: 48, writeBackSeconds: 0 })));
});

test("cache location failure keeps the current location visible", async () => {
  vi.mocked(api.changeCacheLocation).mockRejectedValue(new Error("Disconnect all drives and resolve pending files before changing the cache."));
  render(<App />);
  await waitFor(() => expect((screen.getByRole("button", { name: /Preferences/ }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(screen.getByRole("button", { name: /Preferences/ }));
  fireEvent.change(screen.getByLabelText("New empty cache directory"), { target: { value: "D:\\NewCache" } });
  fireEvent.click(screen.getByRole("button", { name: "Change cache location" }));
  await screen.findByRole("alert");
  expect(screen.getByText(`Current cache: ${snapshot.cacheLocation}`)).toBeDefined();
  expect(api.settings).not.toHaveBeenCalled();
});

test("remote unknown capacity stays separate from pending local files and refresh", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, mounted: true, pending: 1, capacity: { total: null, used: null, free: 0, checkedAt: null, state: "available" } }] });
  vi.mocked(api.refreshDirectoryCache).mockResolvedValue();
  render(<App />);
  fireEvent.mouseEnter(await screen.findByRole("button", { name: "Remote capacity" }));
  await screen.findByText("Remote capacity: — · Used: — · Available: 0 B");
  fireEvent.click(screen.getByRole("button", { name: "Refresh files in Explorer" }));
  await waitFor(() => expect(api.refreshDirectoryCache).toHaveBeenCalledExactlyOnceWith(connection.id));
  expect(screen.getByText("1 pending upload(s)")).toBeDefined();
});

test("wizard uses a chosen letter for its first mount", async () => {
  const connect = await openAuthenticatedFolder();
  fireEvent.change(screen.getByLabelText("Windows drive letter"), { target: { value: "T" } });
  fireEvent.click(connect);
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("draft", "", { letter: "T", folder: "", mode: "" });
  expect(api.mount).toHaveBeenCalledExactlyOnceWith("connected-id");
  expect(screen.getByText("Your files are available at T:.")).toBeDefined();
});

test("occupied letter before confirmation can be corrected without restarting authentication", async () => {
  vi.mocked(api.confirm).mockImplementationOnce(async () => {
    vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, letters: ["T"] });
    throw new Error("letter S: is already in use");
  });
  const connect = await openAuthenticatedFolder();
  fireEvent.click(connect);
  await screen.findByText("letter S: is already in use");
  expect((screen.getByLabelText("Windows drive letter") as HTMLSelectElement).value).toBe("");
  expect((screen.getByRole("button", { name: "Connect drive" }) as HTMLButtonElement).disabled).toBe(true);
  expect(api.mount).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Windows drive letter"), { target: { value: "T" } });
  fireEvent.click(screen.getByRole("button", { name: "Connect drive" }));
  await screen.findByText("Drive connected");
  expect(api.begin).toHaveBeenCalledTimes(1);
  expect(api.confirm).toHaveBeenLastCalledWith("draft", "", { letter: "T", folder: "", mode: "" });
  expect(api.mount).toHaveBeenCalledExactlyOnceWith("connected-id");
});

test("wizard saves a corrected letter on the same connection after a mount failure", async () => {
  vi.mocked(api.mount).mockRejectedValueOnce(new Error("Letter occupied"));
  const connect = await openAuthenticatedFolder();
  fireEvent.click(connect);
  await screen.findByText("Letter occupied");
  await screen.findByText("Configuration saved. Retry uses this connection without creating another. You can also close setup and connect from My drives.");
  fireEvent.change(screen.getByLabelText("Windows drive letter"), { target: { value: "T" } });
  fireEvent.click(await screen.findByRole("button", { name: "Retry connection" }));
  await screen.findByText("Drive connected");
  expect(api.begin).toHaveBeenCalledTimes(1);
  expect(api.confirm).toHaveBeenNthCalledWith(1, "draft", "", { letter: "S", folder: "", mode: "" });
  expect(api.confirm).toHaveBeenNthCalledWith(2, "draft", "", { letter: "T", folder: "", mode: "" });
  expect(api.mount).toHaveBeenNthCalledWith(1, "connected-id");
  expect(api.mount).toHaveBeenNthCalledWith(2, "connected-id");
});

test("uncertain mount recovery locks letter correction and still reuses the saved connection", async () => {
  vi.mocked(api.mount).mockImplementationOnce(async () => {
    vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, id: "connected-id", recovery: true, pending: null }] });
    throw new Error("Mount state unknown");
  });
  const connect = await openAuthenticatedFolder();
  fireEvent.click(connect);
  await screen.findByText("Mount state unknown");
  await waitFor(() => expect((screen.getByLabelText("Windows drive letter") as HTMLSelectElement).disabled).toBe(true));
  expect(screen.getByText("Keep the saved letter while recovery or an operation is pending. Resolve pending files before changing it.")).toBeDefined();
  fireEvent.click(await screen.findByRole("button", { name: "Retry connection" }));
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledTimes(1);
});

test.each(["drive", "ftp"])("edit read-only control follows provider rules (%s)", async (provider) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [...snapshot.providers, ftpProvider], connections: [{ ...connection, provider }] });
  vi.mocked(api.update).mockResolvedValue();
  render(<App />);
  fireEvent.click(await screen.findByText("Edit drive"));
  const mode = screen.getByLabelText("Mount read only") as HTMLInputElement;
  expect(mode.disabled).toBe(provider === "drive");
  if (provider === "drive") expect(screen.getByText("Use Reconfigure account to change Google Drive access mode and update OAuth consent.")).toBeDefined();
  else fireEvent.click(mode);
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  await waitFor(() => expect(api.update).toHaveBeenCalledWith(expect.objectContaining({ readOnly: provider !== "drive" })));
});

test.each(["en", "pt-BR"])("successful access test explains the write-permission limit (%s)", async (language) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection], settings: { ...snapshot.settings, language: language as "en" | "pt-BR" } });
  vi.mocked(api.test).mockResolvedValue();
  render(<App />);
  fireEvent.click(await screen.findByText(language === "en" ? "Test access" : "Testar acesso"));
  await screen.findByText(language === "en"
    ? "Access test succeeded. Listing folders does not verify write permission."
    : "Teste de acesso concluído com sucesso. A listagem de pastas não verifica a permissão de escrita.");
  expect(api.test).toHaveBeenCalledExactlyOnceWith(connection.id);
});

test("failed access test never displays success", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  vi.mocked(api.test).mockRejectedValue(new Error("Account unavailable"));
  render(<App />);
  fireEvent.click(await screen.findByText("Test access"));
  await screen.findByText("Account unavailable");
  expect(screen.queryByText("Access test succeeded. Listing folders does not verify write permission.")).toBeNull();
});

test("wizard distinguishes destination validation from mounting", async () => {
  let confirm!: (id: string) => void;
  let mounted!: () => void;
  vi.mocked(api.confirm).mockReturnValue(new Promise<string>((resolve) => { confirm = resolve; }));
  vi.mocked(api.mount).mockReturnValue(new Promise<void>((resolve) => { mounted = resolve; }));
  const connect = await openAuthenticatedFolder();
  fireEvent.click(connect);
  await screen.findByRole("button", { name: "Validating destination…" });
  expect(api.mount).not.toHaveBeenCalled();
  await act(async () => confirm("connected-id"));
  await screen.findByRole("button", { name: "Mounting drive…" });
  expect(screen.queryByText("Drive connected")).toBeNull();
  await act(async () => mounted());
  await screen.findByText("Drive connected");
});

test("automatic connection errors keep a card warning and details in the alerts tooltip", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, autoMount: true, error: "run the SDrive installer again to prepare virtual drives" }] });
  render(<App />);
  await screen.findByText("Connection needs attention.");
  expect(screen.queryByText("run the SDrive installer again to prepare virtual drives")).toBeNull();
  fireEvent.mouseEnter(screen.getByRole("button", { name: "Alerts and recovery" }));
  await screen.findByText("run the SDrive installer again to prepare virtual drives");
  expect(screen.getByRole("button", { name: "Connect" })).toBeDefined();
  expect(screen.queryByText("Connected", { exact: true })).toBeNull();
});

test("folder listing retries the same draft and path without signing in again", async () => {
  vi.mocked(api.folders).mockRejectedValueOnce(new Error("Temporary folder error"));
  const connect = await openAuthenticatedFolder();
  await screen.findByText(/Temporary folder error/);
  fireEvent.change(screen.getByLabelText("Filter folders"), { target: { value: "keep this filter" } });
  fireEvent.click(screen.getByRole("button", { name: "Retry folder listing" }));
  await screen.findByText("No subfolders here.");
  expect((screen.getByLabelText("Filter folders") as HTMLInputElement).value).toBe("keep this filter");
  expect(api.folders).toHaveBeenNthCalledWith(1, "draft", "");
  expect(api.folders).toHaveBeenNthCalledWith(2, "draft", "");
  expect(api.begin).toHaveBeenCalledTimes(1);
  fireEvent.click(connect);
  await screen.findByText("Drive connected");
});

test("local filtering leaves the destination intact and breadcrumbs return to ancestors and root", async () => {
  vi.mocked(api.folders).mockImplementation(async (_id, path) => path === ""
    ? [{ name: "Documents", path: "Documents" }, { name: "Pictures", path: "Pictures" }]
    : path === "Documents" ? [{ name: "Work", path: "Documents/Work" }] : []);
  await openAuthenticatedFolder();
  fireEvent.change(screen.getByLabelText("Filter folders"), { target: { value: "dOc" } });
  expect(screen.queryByRole("button", { name: "▱ Pictures" })).toBeNull();
  expect(api.folders).toHaveBeenCalledTimes(1);
  fireEvent.change(screen.getByLabelText("Filter folders"), { target: { value: "missing" } });
  expect(screen.getByText("No folders match your filter.")).toBeDefined();
  expect(screen.queryByText("No subfolders here.")).toBeNull();
  expect(api.folders).toHaveBeenCalledTimes(1);
  fireEvent.change(screen.getByLabelText("Filter folders"), { target: { value: "" } });
  fireEvent.click(screen.getByRole("button", { name: "▱ Documents" }));
  fireEvent.click(await screen.findByRole("button", { name: "▱ Work" }));
  await screen.findByText("No subfolders here.");
  expect((screen.getByLabelText("Folder, bucket or share") as HTMLInputElement).value).toBe("Documents/Work");
  expect(screen.getByRole("button", { name: "Work" }).getAttribute("aria-current")).toBe("location");
  fireEvent.click(screen.getByRole("button", { name: "Documents" }));
  await screen.findByRole("button", { name: "▱ Work" });
  expect((screen.getByLabelText("Filter folders") as HTMLInputElement).value).toBe("");
  fireEvent.click(screen.getByRole("button", { name: "Entire storage" }));
  await screen.findByRole("button", { name: "▱ Pictures" });
  expect((screen.getByLabelText("Folder, bucket or share") as HTMLInputElement).value).toBe("");
  fireEvent.click(screen.getByRole("button", { name: "Connect drive" }));
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("draft", "", { letter: "S", folder: "", mode: "" });
});

test("root failure allows a direct destination and unapplied text cannot confirm the previous path", async () => {
  vi.mocked(api.folders).mockRejectedValueOnce(new Error("Root access denied"));
  const connect = await openAuthenticatedFolder();
  const input = screen.getByLabelText("Folder, bucket or share");
  fireEvent.change(input, { target: { value: "allowed/folder" } });
  expect((connect as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText("Destination not applied. Choose Open destination before connecting.")).toBeDefined();
  expect(api.folders).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByText("Enter destination directly"));
  fireEvent.click(screen.getByRole("button", { name: "Open destination" }));
  await screen.findByText("No subfolders here.");
  await waitFor(() => expect((connect as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(connect);
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("draft", "allowed/folder", { letter: "S", folder: "", mode: "" });
});

test("stale folder success and failure cannot replace the latest destination", async () => {
  let resolveOld!: (items: { name: string; path: string }[]) => void;
  let rejectOld!: (error: Error) => void;
  const connect = await openAuthenticatedFolder();
  vi.mocked(api.folders).mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve; }));
  fireEvent.change(screen.getByLabelText("Folder, bucket or share"), { target: { value: "slow" } });
  fireEvent.click(screen.getByRole("button", { name: "Open destination" }));
  await waitFor(() => expect(api.folders).toHaveBeenLastCalledWith("draft", "slow"));
  vi.mocked(api.folders).mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectOld = reject; }));
  fireEvent.click(screen.getByRole("button", { name: "Entire storage" }));
  await waitFor(() => expect(api.folders).toHaveBeenLastCalledWith("draft", ""));
  fireEvent.change(screen.getByLabelText("Folder, bucket or share"), { target: { value: "latest" } });
  fireEvent.click(screen.getByRole("button", { name: "Open destination" }));
  await screen.findByText("No subfolders here.");
  await act(async () => {
    resolveOld([{ name: "Old result", path: "slow/old" }]);
    rejectOld(new Error("Old failure"));
  });
  expect(screen.queryByRole("button", { name: "▱ Old result" })).toBeNull();
  expect(screen.queryByText(/Old failure/)).toBeNull();
  fireEvent.click(connect);
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledWith("draft", "latest", { letter: "S", folder: "", mode: "" });
});

test("listing retries cannot overlap and late results are ignored after closing", async () => {
  vi.mocked(api.folders).mockRejectedValueOnce(new Error("Retry me"));
  await openAuthenticatedFolder();
  let finish!: (items: { name: string; path: string }[]) => void;
  vi.mocked(api.folders).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve; }));
  const retry = screen.getByRole("button", { name: "Retry folder listing" });
  fireEvent.click(retry);
  fireEvent.click(retry);
  await screen.findByText("Loading folders…");
  expect(api.folders).toHaveBeenCalledTimes(2);
  fireEvent.click(screen.getByRole("button", { name: "Close setup" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await act(async () => finish([{ name: "Late", path: "Late" }]));
  expect(screen.queryByText("▱ Late")).toBeNull();
  expect(api.end).toHaveBeenCalledExactlyOnceWith("draft");
});

test("saved setup locks destination navigation and filtering after a mount failure", async () => {
  vi.mocked(api.mount).mockRejectedValueOnce(new Error("Mount failed"));
  fireEvent.click(await openAuthenticatedFolder());
  await screen.findByText("Mount failed");
  for (const label of ["Filter folders", "Folder, bucket or share"]) {
    expect((screen.getByLabelText(label) as HTMLInputElement).disabled).toBe(true);
  }
  for (const name of ["Entire storage", "Open destination"]) {
    expect((screen.getByRole("button", { name }) as HTMLButtonElement).disabled).toBe(true);
  }
  fireEvent.click(screen.getByRole("button", { name: "Retry connection" }));
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledTimes(1);
});

test.each(["en", "pt-BR"])("folder controls and retry feedback are translated (%s)", async (language) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: language as "en" | "pt-BR" } });
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({connectionID: "", id: "draft", busy: false, complete: false, authenticated: true, question: null, error: "" });
  vi.mocked(api.folders).mockRejectedValueOnce(new Error("could not access this folder; check the destination and account permissions"));
  render(<App />);
  fireEvent.click(await screen.findByText(language === "en" ? "Add my first connection" : "Adicionar minha primeira conexão"));
  selectAvailableCategory();
  fireEvent.click(screen.getByText(language === "en" ? "Continue" : "Continuar"));
  fireEvent.click(screen.getByText(language === "en" ? "Sign in with Google Drive" : "Entrar com Google Drive"));
  const retry = await screen.findByRole("button", { name: language === "en" ? "Retry folder listing" : "Tentar listar pastas novamente" });
  expect(screen.getByRole("navigation", { name: language === "en" ? "Folder navigation" : "Navegação de pastas" })).toBeDefined();
  expect(screen.getByLabelText(language === "en" ? "Filter folders" : "Filtrar pastas")).toBeDefined();
  fireEvent.click(retry);
  await screen.findByText(language === "en" ? "No subfolders here." : "Nenhuma subpasta neste local.");
});

test.each([
  ["host", "Server", "bad-host", "enter only the server hostname or IP"],
  ["port", "Port (optional)", "65536", "invalid port"],
  ["pass", "Password", "synthetic-secret", "invalid field: Password"],
])("access errors identify %s, reveal advanced fields and preserve entered values", async (field, label, value, message) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [...snapshot.providers, {
    ...ftpProvider, fields: [...ftpProvider.fields, {file: false, key: "port", label: "Port (optional)", secret: false, required: false, default: "", advanced: true }],
  }], connections: [{ ...connection, provider: "ftp" }] });
  vi.mocked(api.fields).mockResolvedValue({ values: { host: "example.test", user: "original" }, transport: "explicit" });
  vi.mocked(api.begin).mockRejectedValue(new Error(message, { cause: { field, message } }));
  render(<App />);
  fireEvent.click(await screen.findByText("Reconfigure account"));
  await waitFor(() => expect((screen.getByLabelText("Username") as HTMLInputElement).value).toBe("original"));
  const input = screen.getByLabelText(label) as HTMLInputElement;
  fireEvent.change(input, { target: { value } });
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  const error = await screen.findByText(message);
  await waitFor(() => expect(document.activeElement).toBe(input));
  expect(input.value).toBe(value);
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(input.getAttribute("aria-describedby")?.split(" ")).toContain(error.id);
  if (field === "port") expect(input.closest("details")?.open).toBe(true);
  expect((screen.getByLabelText("Username") as HTMLInputElement).value).toBe("original");
  fireEvent.change(input, { target: { value: "corrected" } });
  expect(input.getAttribute("aria-invalid")).toBe("false");
  expect(screen.queryByText(message)).toBeNull();
});

test("destination errors open the direct field, preserve it and prevent confirmation", async () => {
  await openAuthenticatedFolder();
  const input = screen.getByLabelText("Folder, bucket or share") as HTMLInputElement;
  fireEvent.change(input, { target: { value: "../outside" } });
  vi.mocked(api.folders).mockRejectedValueOnce(new Error("path must not contain ..", { cause: { field: "path", message: "path must not contain .." } }));
  fireEvent.click(screen.getByRole("button", { name: "Open destination" }));
  const error = await screen.findByText("path must not contain ..");
  await waitFor(() => expect(document.activeElement).toBe(input));
  expect(input.closest("details")?.open).toBe(true);
  expect(input.value).toBe("../outside");
  expect(input.getAttribute("aria-describedby")?.split(" ")).toContain(error.id);
  expect((screen.getByRole("button", { name: "Connect drive" }) as HTMLButtonElement).disabled).toBe(true);
  expect(api.confirm).not.toHaveBeenCalled();
  fireEvent.change(input, { target: { value: "allowed" } });
  fireEvent.click(screen.getByRole("button", { name: "Open destination" }));
  await waitFor(() => expect((screen.getByRole("button", { name: "Connect drive" }) as HTMLButtonElement).disabled).toBe(false));
});

test("edit path errors remain associated with the entered path", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  vi.mocked(api.update).mockRejectedValue(new Error("path must not contain ..", { cause: { field: "path", message: "path must not contain .." } }));
  render(<App />);
  fireEvent.click(await screen.findByText("Edit drive"));
  fireEvent.click(screen.getByRole("tab", { name: "Files" }));
  const input = screen.getByLabelText("Folder / bucket / share") as HTMLInputElement;
  fireEvent.change(input, { target: { value: "../outside" } });
  fireEvent.click(screen.getByRole("tab", { name: "General" }));
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  const error = await screen.findByText("path must not contain ..");
  expect(input.value).toBe("../outside");
  expect(input.getAttribute("aria-describedby")?.split(" ")).toContain(error.id);
  await waitFor(() => expect(document.activeElement).toBe(input));
});

test.each(["en", "pt-BR"])("URL errors are accessible and preserve the address (%s)", async (language) => {
  const english = language === "en";
  const value = "https://user:synthetic-secret@example.test/dav";
  const message = "url must be an HTTPS URL without embedded credentials";
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot,
    settings: { ...snapshot.settings, language: language as "en" | "pt-BR" },
    providers: [...snapshot.providers, { oauth: false, available: true, id: "webdav", name: "WebDAV", backend: "webdav", transportEncryption: "optional", experimental: false,
      fields: [{ advanced: false, file: false, key: "url", label: "WebDAV HTTPS URL", secret: false, required: true, default: "" }] }],
    connections: [{ ...connection, provider: "webdav" }],
  });
  vi.mocked(api.fields).mockResolvedValue({ values: { url: value }, transport: "" });
  vi.mocked(api.begin).mockRejectedValue(new Error(message, { cause: { field: "url", message } }));
  render(<App />);
  fireEvent.click(await screen.findByText(english ? "Reconfigure account" : "Reconfigurar conta"));
  const input = await screen.findByLabelText(english ? "WebDAV HTTPS URL" : "URL HTTPS do WebDAV") as HTMLInputElement;
  await waitFor(() => expect(input.value).toBe(value));
  fireEvent.click(screen.getByRole("button", { name: english ? "Continue" : "Continuar" }));
  const error = await screen.findByText(english ? message : "url deve ser uma URL HTTPS sem credenciais embutidas");
  await waitFor(() => expect(document.activeElement).toBe(input));
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(input.getAttribute("aria-describedby")?.split(" ")).toContain(error.id);
  expect(input.value).toBe(value);
  expect(error.textContent).not.toContain("synthetic-secret");
});

test("recovery keeps its original folder and disables all breadcrumb destinations", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, recovery: true, path: "Original/library" }] });
  vi.mocked(api.fields).mockResolvedValue({ values: {}, transport: "" });
  vi.mocked(api.begin).mockResolvedValue("recovery");
  vi.mocked(api.status).mockResolvedValue({connectionID: "", id: "recovery", busy: false, complete: false, authenticated: true, question: null, error: "" });
  render(<App />);
  fireEvent.click(await screen.findByText("Reconfigure account"));
  await waitFor(() => expect((screen.getByRole("button", { name: "Sign in with Google Drive" }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(await screen.findByRole("button", { name: "Sign in with Google Drive" }));
  await screen.findByRole("heading", { name: "Choose what to connect" });
  await waitFor(() => expect(api.folders).toHaveBeenCalledExactlyOnceWith("recovery", "Original/library"));
  for (const name of ["Entire storage", "Original", "library"]) {
    expect((screen.getByRole("button", { name }) as HTMLButtonElement).disabled).toBe(true);
  }
  expect(screen.queryByLabelText("Filter folders")).toBeNull();
  expect(screen.queryByLabelText("Folder, bucket or share")).toBeNull();
  expect(screen.queryByRole("button", { name: "↑ Parent folder" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Connect drive" }));
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("recovery", "Original/library", { letter: "S", folder: "", mode: "" });
});

test("operation stages use the saved Portuguese language", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" }, connections: [{ ...connection, busy: true, operation: "waitingUploads", pending: null }] });
  render(<App />);
  await screen.findByText("Aguardando uploads…");
  expect(screen.queryByText("Conectado", { exact: true })).toBeNull();
});

test("engine restart reports failure, prevents duplicate requests and preserves recovery on retry", async () => {
  const recovering = { ...connection, recovery: true, pending: null };
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [recovering], engineError: "Engine stopped for test" });
  let reject!: (error: Error) => void;
  vi.mocked(api.restart).mockReturnValueOnce(new Promise<void>((_, failure) => { reject = failure; }));
  render(<App />);
  const restart = await screen.findByRole("button", { name: "Restart engine" });
  fireEvent.click(restart);
  expect((restart as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(restart);
  expect(api.restart).toHaveBeenCalledTimes(1);
  await act(async () => reject(new Error("Restart failed for test")));
  await screen.findByText("Restart failed for test");
  expect(screen.queryByText("Engine restarted. Recover pending connections.")).toBeNull();
  expect((restart as HTMLButtonElement).disabled).toBe(false);
  vi.mocked(api.restart).mockResolvedValue();
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [recovering] });
  fireEvent.click(restart);
  await screen.findByText("Engine restarted. Recover pending connections.");
  await waitFor(() => expect(screen.queryByRole("button", { name: "Restart engine" })).toBeNull());
  expect(api.restart).toHaveBeenCalledTimes(2);
  expect(screen.getByRole("button", { name: "Recover" })).toBeDefined();
  expect(screen.queryByText("Connected", { exact: true })).toBeNull();
});

test("SSH identity probing retries and requires fresh trust after changing the server", async () => {
  const sftp = { ...ftpProvider, id: "sftp", name: "SFTP", backend: "sftp", transportEncryption: "required", fields: [
    ...ftpProvider.fields,
    { advanced: true, file: false, key: "port", label: "Port (optional)", secret: false, required: false, default: "" },
  ] };
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [...snapshot.providers, sftp] });
  vi.mocked(api.probe).mockRejectedValueOnce(new Error("SSH probe failed for test"));
  vi.mocked(api.begin).mockResolvedValue("ssh-session");
  vi.mocked(api.status).mockResolvedValue({ id: "ssh-session", busy: true, authenticated: false, complete: false, question: null, error: "", connectionID: "" });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  selectAvailableCategory();
  fireEvent.change(screen.getByLabelText("Connection type"), { target: { value: "sftp" } });
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  const host = screen.getByLabelText("Server");
  fireEvent.change(host, { target: { value: "first.example.test" } });
  fireEvent.change(screen.getByLabelText("Username"), { target: { value: "synthetic-user" } });
  const probe = screen.getByRole("button", { name: "Verify SSH identity" });
  fireEvent.click(probe);
  await screen.findByText("SSH probe failed for test");
  expect(screen.queryByLabelText("I verified the fingerprint and trust this server")).toBeNull();
  vi.mocked(api.probe).mockResolvedValue({ key: "first-key", fingerprint: "first-fingerprint" });
  fireEvent.click(probe);
  await screen.findByText("first-fingerprint");
  expect(api.probe).toHaveBeenLastCalledWith("first.example.test", "");
  expect((screen.getByLabelText("I verified the fingerprint and trust this server") as HTMLInputElement).checked).toBe(false);
  expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.click(screen.getByLabelText("I verified the fingerprint and trust this server"));
  fireEvent.change(host, { target: { value: "second.example.test" } });
  fireEvent.change(screen.getByLabelText("Port (optional)"), { target: { value: "2222" } });
  expect(screen.queryByText("first-fingerprint")).toBeNull();
  expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(true);
  vi.mocked(api.probe).mockResolvedValue({ key: "second-key", fingerprint: "second-fingerprint" });
  fireEvent.click(probe);
  await screen.findByText("second-fingerprint");
  expect(api.probe).toHaveBeenLastCalledWith("second.example.test", "2222");
  const trust = screen.getByLabelText("I verified the fingerprint and trust this server") as HTMLInputElement;
  expect(trust.checked).toBe(false);
  fireEvent.click(trust);
  fireEvent.change(screen.getByLabelText("Port (optional)"), { target: { value: "2223" } });
  expect(screen.queryByText("second-fingerprint")).toBeNull();
  expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(true);
  vi.mocked(api.probe).mockResolvedValue({ key: "accepted-key", fingerprint: "accepted-fingerprint" });
  fireEvent.click(probe);
  await screen.findByText("accepted-fingerprint");
  expect(api.probe).toHaveBeenLastCalledWith("second.example.test", "2223");
  expect((screen.getByLabelText("I verified the fingerprint and trust this server") as HTMLInputElement).checked).toBe(false);
  fireEvent.click(screen.getByLabelText("I verified the fingerprint and trust this server"));
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await waitFor(() => expect(api.begin).toHaveBeenCalledWith(expect.objectContaining({
    hostKey: "accepted-key", values: { host: "second.example.test", port: "2223", user: "synthetic-user" },
  })));
});

test.each([
  ["validating", "Validating account…"],
  ["mounting", "Mounting drive…"],
  ["checkingUploads", "Checking pending uploads…"],
  ["waitingUploads", "Waiting for uploads…"],
  ["unmounting", "Unmounting drive…"],
] as const)("drive displays backend operation %s without declaring unknown uploads safe", async (operation, text) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, busy: true, operation, pending: null }] });
  render(<App />);
  await screen.findByText(text);
  expect(screen.queryByText("Connected", { exact: true })).toBeNull();
  expect((screen.getByRole("button", { name: "Connect" }) as HTMLButtonElement).disabled).toBe(true);
});


test("a locked snapshot removes management even if the native lock event was missed", async () => {
  let visibility!: (visible:boolean) => void;
  vi.mocked(api.onDesktopVisibility).mockImplementation((callback) => {visibility = callback;return () => {};});
  render(<App />);
  await screen.findByText("One letter. All your files.");
  vi.mocked(api.accessStatus).mockResolvedValue({...access,enabled:true,locked:true});
  vi.mocked(api.snapshot).mockResolvedValue({...snapshot,access:{...access,enabled:true,locked:true}});
  act(() => {visibility(false);visibility(true);});
  await screen.findByRole("heading",{name:"SDrive is locked"});
  expect(screen.queryByRole("button",{name:"+ New connection"})).toBeNull();
});


test("drive cards hide details behind seven labeled icons without opening a dialog", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, path: "Team files", mounted: true,
    transportSecurity: { state: "encrypted", protocol: "https", source: "configuration", checkedAt: null },
  }] });
  render(<App />);
  const group = await screen.findByRole("group", { name: "Drive details" });
  expect(within(group).getAllByRole("button")).toHaveLength(7);
  expect(screen.queryByRole("tooltip")).toBeNull();
  expect(screen.queryByText(/Approximate cache used/)).toBeNull();
  expect(screen.queryByText("Google Drive · Team files")).toBeNull();
  const topics = [
    ["Remote destination", "Google Drive · Team files"],
    ["Connection options", "Automatic mounting disabled"],
    ["Upload details", "Upload activity is currently unknown."],
    ["Local cache", "Approximate cache used: 0 B · Pending file sizes: 0 B"],
    ["Remote capacity", "Remote capacity is currently unknown."],
    ["Connection security", "Traffic encrypted · HTTPS"],
    ["Alerts and recovery", "No alerts or recovery required."],
  ];
  for (const [label, detail] of topics) {
    const icon = within(group).getByRole("button", { name: label });
    fireEvent.mouseEnter(icon);
    const tooltip = screen.getByRole("tooltip");
    expect(tooltip.textContent).toContain(detail);
    expect(icon.getAttribute("aria-describedby")).toBe(tooltip.id);
    expect(screen.getAllByRole("tooltip")).toHaveLength(1);
    expect(screen.queryByRole("dialog")).toBeNull();
  }
});

test("drive tooltips support focus, Escape, touch clicks and dismissal outside", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  render(<App />);
  const icon = await screen.findByRole("button", { name: "Connection security" });
  act(() => icon.focus());
  expect(screen.getByRole("tooltip").textContent).toContain("unavailable while disconnected");
  fireEvent.keyDown(document, { key: "Escape" });
  expect(screen.queryByRole("tooltip")).toBeNull();
  expect(document.activeElement).toBe(icon);
  expect(icon.getAttribute("aria-describedby")).toBeNull();
  fireEvent.click(icon);
  expect(screen.getByRole("tooltip")).toBeDefined();
  act(() => screen.getByRole("button", { name: "Connect" }).focus());
  expect(screen.queryByRole("tooltip")).toBeNull();
  fireEvent.click(icon);
  fireEvent.pointerDown(document.body);
  expect(screen.queryByRole("tooltip")).toBeNull();
  act(() => icon.focus());
  act(() => icon.blur());
  expect(screen.queryByRole("tooltip")).toBeNull();
});

test("hovered tooltips remain open while crossing into their selectable content", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  render(<App />);
  const icon = await screen.findByRole("button", { name: "Local cache" });
  vi.useFakeTimers();
  try {
    fireEvent.mouseEnter(icon);
    fireEvent.mouseLeave(screen.getByRole("group", { name: "Drive details" }));
    act(() => vi.advanceTimersByTime(60));
    fireEvent.mouseEnter(screen.getByRole("tooltip"));
    act(() => vi.advanceTimersByTime(200));
    expect(screen.getByRole("tooltip").textContent).toContain("Pending sizes are whole local files");
    fireEvent.mouseLeave(screen.getByRole("tooltip"));
    act(() => vi.advanceTimersByTime(150));
    expect(screen.queryByRole("tooltip")).toBeNull();
  } finally { vi.useRealTimers(); }
});

test("only one drive tooltip opens and its content follows refreshed snapshots", async () => {
  let current: Snapshot = { ...snapshot, connections: [
    { ...connection, name: "First drive", path: "First library" },
    { ...connection, id: "second", name: "Second drive", path: "Second library" },
  ] };
  vi.mocked(api.snapshot).mockImplementation(async () => current);
  vi.useFakeTimers();
  try {
    render(<App />);
    await act(async () => {});
    const icons = screen.getAllByRole("button", { name: "Remote destination" });
    fireEvent.mouseEnter(icons[0]);
    expect(screen.getByRole("tooltip").textContent).toContain("First library");
    fireEvent.keyDown(document, { key: "Escape" });
    fireEvent.mouseLeave(screen.getAllByRole("group", { name: "Drive details" })[0]);
    fireEvent.mouseEnter(icons[1]);
    act(() => vi.advanceTimersByTime(150));
    expect(screen.getAllByRole("tooltip")).toHaveLength(1);
    expect(screen.getByRole("tooltip").textContent).toContain("Second library");
    current = { ...current, connections: current.connections.map((c) => c.id === "second" ? { ...c, path: "Updated library" } : c) };
    await act(async () => { await vi.advanceTimersByTimeAsync(3000); });
    expect(screen.getByRole("tooltip").textContent).toContain("Updated library");
    expect(screen.getByRole("tooltip").textContent).not.toContain("First library");
  } finally { vi.useRealTimers(); }
});


test.each(["en", "pt-BR"] as const)("edit tabs group settings and support keyboard navigation (%s)", async (language) => {
  const english = language === "en";
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection], settings: { ...snapshot.settings, language } });
  render(<App />);
  fireEvent.click(await screen.findByText(english ? "Edit drive" : "Editar unidade"));
  const general = screen.getByRole("tab", { name: english ? "General" : "Geral" });
  const files = screen.getByRole("tab", { name: english ? "Files" : "Arquivos" });
  const network = screen.getByRole("tab", { name: english ? "Network" : "Rede" });
  expect(screen.getAllByRole("tab")).toHaveLength(3);
  expect(general.getAttribute("aria-selected")).toBe("true");
  expect(screen.getByRole("tabpanel").id).toBe(general.getAttribute("aria-controls"));
  expect(within(screen.getByRole("tabpanel")).getByLabelText(english ? "Drive name" : "Nome da unidade")).toBeTruthy();
  fireEvent.keyDown(general, { key: "ArrowRight" });
  expect(document.activeElement).toBe(files);
  expect(screen.getByRole("tabpanel").id).toBe(files.getAttribute("aria-controls"));
  const panel = screen.getByRole("tabpanel");
  expect(panel.querySelector('[data-field="path"]')).toBeTruthy();
  expect(panel.querySelector('[data-field="directoryCacheSeconds"]')).toBeTruthy();
  expect(panel.querySelector('[data-field="bandwidth"]')).toBeNull();
  fireEvent.keyDown(files, { key: "End" });
  expect(document.activeElement).toBe(network);
  expect(screen.getByRole("tabpanel").querySelector('[data-field="bandwidth"]')).toBeTruthy();
  fireEvent.keyDown(network, { key: "ArrowRight" });
  expect(document.activeElement).toBe(general);
  fireEvent.keyDown(general, { key: "ArrowLeft" });
  expect(document.activeElement).toBe(network);
  fireEvent.keyDown(network, { key: "Home" });
  expect(document.activeElement).toBe(general);
});

test("editing preserves values across tabs and saves them together", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  vi.mocked(api.update).mockResolvedValue();
  render(<App />);
  fireEvent.click(await screen.findByText("Edit drive"));
  fireEvent.change(screen.getByLabelText("Drive name"), { target: { value: "Renamed drive" } });
  fireEvent.click(screen.getByLabelText("Connect automatically when SDrive starts"));
  fireEvent.click(screen.getByRole("tab", { name: "Files" }));
  fireEvent.change(screen.getByLabelText("Folder / bucket / share"), { target: { value: "Documents" } });
  fireEvent.change(screen.getByLabelText("Directory metadata lifetime (seconds)"), { target: { value: "600" } });
  fireEvent.click(screen.getByRole("tab", { name: "Network" }));
  fireEvent.change(screen.getByLabelText("Download limit (KiB/s)"), { target: { value: "512" } });
  fireEvent.click(screen.getByRole("tab", { name: "General" }));
  expect((screen.getByLabelText("Drive name") as HTMLInputElement).value).toBe("Renamed drive");
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  await waitFor(() => expect(api.update).toHaveBeenCalledWith(expect.objectContaining({
    name: "Renamed drive", autoMount: true, path: "Documents", directoryCacheSeconds: 600, downloadLimit: 512 * 1024,
  })));
});

test("server errors reveal the network tab and focus the invalid field", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  vi.mocked(api.update).mockRejectedValue(new Error("Invalid bandwidth", { cause: { field: "bandwidth", message: "Invalid bandwidth" } }));
  render(<App />);
  fireEvent.click(await screen.findByText("Edit drive"));
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  await screen.findByText("Invalid bandwidth");
  await waitFor(() => expect(screen.getByRole("tab", { name: "Network" }).getAttribute("aria-selected")).toBe("true"));
  await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText("Download limit (KiB/s)")));
  fireEvent.click(screen.getByRole("tab", { name: "General" }));
  expect(screen.getByRole("tab", { name: "General" }).getAttribute("aria-selected")).toBe("true");
});

test.each(["name", "directoryCacheSeconds", "bandwidth"])("native validation reveals a hidden invalid field (%s)", async (field) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  render(<App />);
  fireEvent.click(await screen.findByText("Edit drive"));
  const dialog = screen.getByRole("dialog");
  const input = dialog.querySelector<HTMLInputElement>(`[data-field="${field}"]`)!;
  const panel = input.closest<HTMLElement>('[role="tabpanel"]')!;
  fireEvent.change(input, { target: { value: field === "name" ? "" : "-1" } });
  fireEvent.click(screen.getByRole("tab", { name: field === "name" ? "Files" : "General" }));
  expect(panel.hidden).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  expect(panel.hidden).toBe(false);
  expect(api.update).not.toHaveBeenCalled();
});

test("multiple invalid panels keep the first invalid field visible", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [connection] });
  render(<App />);
  fireEvent.click(await screen.findByText("Edit drive"));
  fireEvent.change(screen.getByLabelText("Drive name"), { target: { value: "" } });
  fireEvent.click(screen.getByRole("tab", { name: "Files" }));
  fireEvent.change(screen.getByLabelText("Directory metadata lifetime (seconds)"), { target: { value: "-1" } });
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  expect(screen.getByRole("tab", { name: "General" }).getAttribute("aria-selected")).toBe("true");
  expect(api.update).not.toHaveBeenCalled();
});
