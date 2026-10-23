import React from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import {
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
  error: "",
};
const snapshot = {
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
