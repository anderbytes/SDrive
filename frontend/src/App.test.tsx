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

vi.mock("./api", () => ({
  api: {
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
  vi.mocked(api.snapshot).mockResolvedValue(snapshot);
  vi.mocked(api.end).mockResolvedValue();
  vi.mocked(api.folders).mockResolvedValue([]);
  vi.mocked(api.confirm).mockResolvedValue("connected-id");
  vi.mocked(api.mount).mockResolvedValue();
});
afterEach(() => cleanup());

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
  fireEvent.change(code, { target: { value: "123456" } });
  fireEvent.click(screen.getByText("Continue"));
  await waitFor(() => expect(api.answer).toHaveBeenCalledWith("mfa", "123456"));
  expect(localStorage.length).toBe(0);
  expect(sessionStorage.length).toBe(0);
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
  expect(api.confirm).toHaveBeenCalledWith("draft", "Documents");
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
    (screen.getByText("Sign in with Google Drive") as HTMLButtonElement)
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
  fireEvent.click(screen.getByText("Continue"));
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
  expect(api.confirm).toHaveBeenLastCalledWith("draft", "");
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
