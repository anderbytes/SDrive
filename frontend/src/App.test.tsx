import React from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { App } from "./App";
import { api, type ConnectionView } from "./api";

vi.mock("@wailsio/runtime", () => ({ Call: { ByName: vi.fn() } }));

vi.mock("./api", async (importOriginal) => ({
  ...await importOriginal<typeof import("./api")>(),
  api: {
    about: vi.fn(),
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
    restart: vi.fn(),
    restoreSettings: vi.fn(),
    verifyRecovery: vi.fn(),
    retryCleanup: vi.fn(),
  },
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
  mounted: false,
  busy: false,
  operation: "",
  pending: 0,
  uploads: null,
  cache: { checkedAt: null, pendingBytes: 0, usedBytes: 0, error: "" },
  error: "",
};
const snapshot = {
  freeBytes: null,
  providers: [
    {
      id: "drive",
      name: "Google Drive",
      backend: "drive",
      experimental: false,
      fields: [],
      oauth: true,
      available: true,
    },
  ],
  connections: [] as ConnectionView[],
  settings: { autoStart: false, cacheGiB: 2 },
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
  vi.mocked(api.desktopVisible).mockResolvedValue(true);
  vi.mocked(api.onDesktopVisibility).mockReturnValue(() => {});
  vi.mocked(api.snapshot).mockResolvedValue(snapshot);
  vi.mocked(api.end).mockResolvedValue();
  vi.mocked(api.folders).mockResolvedValue([]);
  vi.mocked(api.confirm).mockResolvedValue("connected-id");
  vi.mocked(api.mount).mockResolvedValue();
});
afterEach(() => cleanup());

test("About shows actual versions and unavailable components, and returns focus on close", async () => {
  vi.mocked(api.about).mockResolvedValue({ appVersion: "0.1.0", rcloneVersion: "v-running", windowsVersion: "10.0.26100", winFspVersion: "", webView2Version: "123.4", architecture: "amd64" });
  render(<App />);
  const button = screen.getByRole("button", { name: "About SDrive" });
  button.focus();
  fireEvent.click(button);
  await screen.findByText("v-running");
  expect(screen.getByRole("dialog").textContent).toContain("Unavailable or not installed");
  fireEvent.keyDown(document, { key: "Escape" });
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

test("dialog traps keyboard focus and Escape restores the opener", async () => {
  render(<App />);
  const opener = await screen.findByText("Add my first connection");
  opener.focus();
  fireEvent.click(opener);
  const first = screen.getByRole("button", { name: "Close setup" });
  expect(document.activeElement).toBe(first);
  opener.focus();
  fireEvent.keyDown(document, { key: "Tab" });
  expect(document.activeElement).toBe(first);
  fireEvent.keyDown(document, { key: "Tab", shiftKey: true });
  expect(document.activeElement).toBe(
    screen.getByRole("button", { name: "Continue" }),
  );
  fireEvent.keyDown(document, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(document.activeElement).toBe(opener);
});

test("empty state opens accessible connection form", async () => {
  render(<App />);
  await screen.findByText("One letter. All your files.");
  fireEvent.click(screen.getByText("Add my first connection"));
  expect(screen.getByRole("dialog").getAttribute("aria-modal")).toBe("true");
  expect(screen.getByLabelText("Drive name")).toBeDefined();
});
test("cancel OAuth cleans up draft", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({
    id: "draft",
    busy: true,
    complete: false,
    question: null,
    error: "",
  });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
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
    id: "mfa",
    busy: false,
    complete: false,
    error: "",
    question: {
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
  fireEvent.change(screen.getByLabelText("Drive name"), {
    target: { value: "Conta" },
  });
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  const code = await screen.findByLabelText("MFA code");
  expect(code.getAttribute("type")).toBe("password");
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

const ftpProvider = {
  id: "ftp", name: "FTP / FTPS", backend: "ftp", experimental: false, available: true,
  fields: [
    { key: "host", label: "Server", secret: false, required: true, default: "" },
    { key: "user", label: "Username", secret: false, required: true, default: "" },
    { key: "pass", label: "Password", secret: true, required: false, default: "" },
  ],
};

test.each(["explicit", "implicit", "plain"])("FTP renewal restores %s and requires fresh plain FTP consent", async (transport) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [...snapshot.providers, ftpProvider], connections: [{ ...connection, provider: "ftp", recovery: true }] });
  vi.mocked(api.fields).mockResolvedValue({ values: { host: "example.test", user: "original" }, transport });
  vi.mocked(api.begin).mockResolvedValue("renew");
  vi.mocked(api.status).mockResolvedValue({ id: "renew", busy: true, complete: false, question: null, error: "" });
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

test("existing custom OAuth connection can still renew when project sign-in is unavailable", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, providers: [{ ...snapshot.providers[0], available: false }], connections: [connection] });
  vi.mocked(api.fields).mockResolvedValue({ values: { client_id: "custom-public-id" }, transport: "" });
  vi.mocked(api.begin).mockResolvedValue("custom");
  vi.mocked(api.status).mockResolvedValue({ id: "custom", busy: true, complete: false, question: null, error: "" });
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
  vi.mocked(api.status).mockResolvedValue({
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
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  fireEvent.click(await screen.findByRole("button", { name: "▱ Documents" }));
  await waitFor(() =>
    expect(api.folders).toHaveBeenCalledWith("draft", "Documents"),
  );
  fireEvent.click(screen.getByText("Connect drive"));
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledWith("draft", "Documents", "S");
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
  vi.mocked(api.status).mockResolvedValue({
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

test("missing project OAuth is explained without asking the user for Client ID", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    providers: [
      {
        ...snapshot.providers[0],
        available: false,
        notice: "Project sign-in is not ready yet",
      },
    ],
  });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  fireEvent.click(screen.getByText("Continue"));
  expect(screen.getByText("Project sign-in is not ready yet")).toBeDefined();
  expect(
    (screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement)
      .disabled,
  ).toBe(true);
  expect(screen.queryByText(/Client ID/)).toBeNull();
});

test("cancelling after authentication keeps the draft unconfirmed", async () => {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({
    id: "draft",
    busy: false,
    complete: false,
    authenticated: true,
    question: null,
    error: "",
  });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  await screen.findByText("Choose what to connect");
  fireEvent.click(screen.getByText("Cancel"));
  await waitFor(() => expect(api.end).toHaveBeenCalledWith("draft"));
  expect(api.confirm).not.toHaveBeenCalled();
});

test("GCS file picker enables service-account access without project OAuth", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot,
    providers: [
      ...snapshot.providers,
      {
        id: "gcs",
        name: "Google Cloud Storage",
        backend: "google cloud storage",
        experimental: false,
        oauth: true,
        available: false,
        fields: [
          {
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
  fireEvent.change(screen.getByLabelText("Connection type"), {
    target: { value: "gcs" },
  });
  expect(screen.getByText("Browser sign-in is unavailable. Continue to select a service account JSON file.")).toBeDefined();
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
  vi.mocked(api.status).mockResolvedValue({
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
  vi.mocked(api.status).mockResolvedValue({
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
  expect(api.confirm).toHaveBeenLastCalledWith("draft", "", "S");
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
  expect(screen.getByText("1 uploading · 2 queued · 0 cache errors")).toBeDefined();
  expect(screen.getByText("Approximate cache used: — · Pending file sizes: —")).toBeDefined();
  expect(screen.getByRole("button", { name: "Disconnect" })).toBeDefined();
});

test("recovery explains the preserved destination and measured cache", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({
    ...snapshot, freeBytes: 10 * 1024 ** 3,
    connections: [{ ...connection, recovery: true, path: "Original library", pending: 2,
      cache: { checkedAt: "2026-10-23T12:00:00Z", usedBytes: 1024, pendingBytes: 2048, error: "" },
    }],
  });
  render(<App />);
  await screen.findByText("Preserved destination: Google Drive · Original library", { exact: false });
  expect(screen.getByText(/Recover using the original account and destination/)).toBeDefined();
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
  fireEvent.click(await screen.findByRole("button", { name: /Preferences/ }));
  fireEvent.change(screen.getByLabelText("Language"), {
    target: { value: "pt-BR" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByRole("heading", { name: "Minhas unidades" });
  expect(api.settings).toHaveBeenCalledWith({
    autoStart: false,
    cacheGiB: 2,
    language: "pt-BR",
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
          {
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
    id: "draft",
    busy: false,
    complete: false,
    error: "",
    question: {
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
  fireEvent.click(screen.getByRole("button", { name: "Exit safely" }));
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

test.each(["en", "pt-BR"])("no letters blocks both creation entries (%s)", async (language) => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, letters: [], settings: { ...snapshot.settings, language: language as "en" | "pt-BR" } });
  render(<App />);
  const english = language === "en";
  await screen.findByText(english
    ? "No drive letters are available. Free a letter between D and Z before creating a connection."
    : "Não há letras de unidade disponíveis. Libere uma letra entre D e Z antes de criar uma conexão.");
  const create = await screen.findByRole("button", { name: english ? "+ New connection" : "+ Nova conexão" });
  expect((create as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByRole("button", { name: english ? "Add my first connection" : "Adicionar minha primeira conexão" }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText(english
    ? "No drive letters are available. Free a letter between D and Z before creating a connection."
    : "Não há letras de unidade disponíveis. Libere uma letra entre D e Z antes de criar uma conexão.")).toBeDefined();
  fireEvent.click(create);
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(api.begin).not.toHaveBeenCalled();
});

test("letters becoming unavailable blocks authentication in an open wizard", async () => {
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, letters: [] });
  await waitFor(() => expect((screen.getByRole("button", { name: "Sign in with Google Drive" }) as HTMLButtonElement).disabled).toBe(true), { timeout: 4000 });
  expect(api.begin).not.toHaveBeenCalled();
});

async function openAuthenticatedFolder() {
  vi.mocked(api.begin).mockResolvedValue("draft");
  vi.mocked(api.status).mockResolvedValue({ id: "draft", busy: false, complete: false, authenticated: true, question: null, error: "" });
  render(<App />);
  fireEvent.click(await screen.findByText("Add my first connection"));
  fireEvent.click(screen.getByText("Continue"));
  fireEvent.click(screen.getByText("Sign in with Google Drive"));
  const connect = await screen.findByRole("button", { name: "Connect drive" });
  await waitFor(() => expect((connect as HTMLButtonElement).disabled).toBe(false));
  return connect;
}

test("wizard uses a chosen letter for its first mount", async () => {
  const connect = await openAuthenticatedFolder();
  fireEvent.change(screen.getByLabelText("Windows drive letter"), { target: { value: "T" } });
  fireEvent.click(connect);
  await screen.findByText("Drive connected");
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("draft", "", "T");
  expect(api.mount).toHaveBeenCalledExactlyOnceWith("connected-id");
  expect(screen.getByText("Your files are available on Windows drive T:.")).toBeDefined();
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
  expect(api.confirm).toHaveBeenLastCalledWith("draft", "", "T");
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
  expect(api.confirm).toHaveBeenNthCalledWith(1, "draft", "", "S");
  expect(api.confirm).toHaveBeenNthCalledWith(2, "draft", "", "T");
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

test("automatic connection errors are visible on the saved drive card", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, autoMount: true, error: "run the SDrive installer again to prepare virtual drives" }] });
  render(<App />);
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
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("draft", "", "S");
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
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("draft", "allowed/folder", "S");
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
  expect(api.confirm).toHaveBeenCalledWith("draft", "latest", "S");
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
  vi.mocked(api.status).mockResolvedValue({ id: "draft", busy: false, complete: false, authenticated: true, question: null, error: "" });
  vi.mocked(api.folders).mockRejectedValueOnce(new Error("could not access this folder; check the destination and account permissions"));
  render(<App />);
  fireEvent.click(await screen.findByText(language === "en" ? "Add my first connection" : "Adicionar minha primeira conexão"));
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
    ...ftpProvider, fields: [...ftpProvider.fields, { key: "port", label: "Port (optional)", secret: false, required: false, default: "", advanced: true }],
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
  expect(input.getAttribute("aria-describedby")).toBe(error.id);
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
  expect(input.getAttribute("aria-describedby")).toBe(error.id);
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
  const input = screen.getByLabelText("Folder / bucket / share") as HTMLInputElement;
  fireEvent.change(input, { target: { value: "../outside" } });
  fireEvent.click(screen.getByRole("button", { name: "Save changes" }));
  const error = await screen.findByText("path must not contain ..");
  expect(input.value).toBe("../outside");
  expect(input.getAttribute("aria-describedby")).toBe(error.id);
  await waitFor(() => expect(document.activeElement).toBe(input));
});

test.each(["en", "pt-BR"])("URL errors are accessible and preserve the address (%s)", async (language) => {
  const english = language === "en";
  const value = "https://user:synthetic-secret@example.test/dav";
  const message = "url must be an HTTPS URL without embedded credentials";
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot,
    settings: { ...snapshot.settings, language: language as "en" | "pt-BR" },
    providers: [...snapshot.providers, { id: "webdav", name: "WebDAV", backend: "webdav", experimental: false,
      fields: [{ key: "url", label: "WebDAV HTTPS URL", secret: false, required: true, default: "" }] }],
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
  expect(input.getAttribute("aria-describedby")).toBe(error.id);
  expect(input.value).toBe(value);
  expect(error.textContent).not.toContain("synthetic-secret");
});

test("recovery keeps its original folder and disables all breadcrumb destinations", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, connections: [{ ...connection, recovery: true, path: "Original/library" }] });
  vi.mocked(api.fields).mockResolvedValue({ values: {}, transport: "" });
  vi.mocked(api.begin).mockResolvedValue("recovery");
  vi.mocked(api.status).mockResolvedValue({ id: "recovery", busy: false, complete: false, authenticated: true, question: null, error: "" });
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
  expect(api.confirm).toHaveBeenCalledExactlyOnceWith("recovery", "Original/library", "S");
});

test("operation stages use the saved Portuguese language", async () => {
  vi.mocked(api.snapshot).mockResolvedValue({ ...snapshot, settings: { ...snapshot.settings, language: "pt-BR" }, connections: [{ ...connection, busy: true, operation: "waitingUploads", pending: null }] });
  render(<App />);
  await screen.findByText("Aguardando uploads…");
  expect(screen.queryByText("Conectado", { exact: true })).toBeNull();
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
