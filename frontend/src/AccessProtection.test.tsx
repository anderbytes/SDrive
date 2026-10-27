import { StrictMode } from "react";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { AccessBoundary, AccessSettings } from "./AccessProtection";
import { FileAccessInspection } from "./FileAccessInspection";
import { api, type AccessStatus } from "./api";
import { LanguageProvider, languages, translate, type Language } from "./i18n";

vi.mock("./api", () => ({ api: {
  end: vi.fn(), accessStatus: vi.fn(), onAccessChanged: vi.fn(), configureAccess: vi.fn(), disableAccess: vi.fn(), unlockAccess: vi.fn(), recoverAccess: vi.fn(), exit: vi.fn(), inspectFileAccess: vi.fn(),
} }));
Object.defineProperties(HTMLDialogElement.prototype, {
  showModal: { configurable: true, value() { this.setAttribute("open", ""); } },
  close: { configurable: true, value() { this.removeAttribute("open"); } },
});
const status: AccessStatus = { theme: "system", setupPending: false, enabled:true, locked:false,language:"en", policy:{idleMinutes:15,disconnectIdle:false,reconnect:false},retryAt:"0001-01-01T00:00:00Z",error:"",warning:"" };
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(api.accessStatus).mockResolvedValue(status);
  vi.mocked(api.onAccessChanged).mockReturnValue(() => {});
});
afterEach(cleanup);

test("startup spinner stops on failure and returns only during a retry", async () => {
  let reject!: (error: Error) => void;
  vi.mocked(api.accessStatus).mockReturnValueOnce(new Promise((_resolve, fail) => { reject = fail; }));
  const view = render(<LanguageProvider><AccessBoundary>{() => <p>management</p>}</AccessBoundary></LanguageProvider>);
  expect(screen.getByRole("status").querySelector(".spinner-large")).not.toBeNull();
  expect(screen.queryByRole("button", { name: "Try again" })).toBeNull();
  await act(async () => reject(new Error("Startup unavailable")));
  expect(screen.getByRole("alert").textContent).toBe("Startup unavailable");
  expect(view.container.querySelector(".spinner")).toBeNull();
  let resolve!: (next: AccessStatus) => void;
  vi.mocked(api.accessStatus).mockReturnValueOnce(new Promise((done) => { resolve = done; }));
  fireEvent.click(screen.getByRole("button", { name: "Try again" }));
  expect(view.container.querySelector(".spinner-large")).not.toBeNull();
  await act(async () => resolve(status));
  expect(screen.getByText("management")).toBeDefined();
  expect(view.container.querySelector(".spinner")).toBeNull();
});

test.each(["cancelled", "failed"])("file inspection removes the spinner when %s", async (outcome) => {
  let finish!: () => void;
  vi.mocked(api.inspectFileAccess).mockImplementationOnce(() => new Promise((resolve, reject) => {
    finish = () => outcome === "failed" ? reject(new Error("Inspection unavailable")) : resolve({ state: "cancelled", path: "", checkedAt: "", processes: [] });
  }));
  const view = render(<LanguageProvider><FileAccessInspection connectionID="drive" disabled={false} onRun={async (work) => { await work(); }} /></LanguageProvider>);
  fireEvent.click(screen.getByRole("button", { name: "Select mounted file" }));
  expect(screen.getByRole("status").querySelector(".spinner")).not.toBeNull();
  await act(async () => finish());
  expect(view.container.querySelector(".spinner")).toBeNull();
  expect(screen.getByRole("button", { name: "Select mounted file" }).hasAttribute("disabled")).toBe(false);
});

test.each(Object.keys(languages) as Language[])("locked startup retains the saved %s language", async (language) => {
  vi.mocked(api.accessStatus).mockResolvedValue({ ...status, locked: true, language });
  render(<LanguageProvider><AccessBoundary>{() => <p>private management</p>}</AccessBoundary></LanguageProvider>);
  await screen.findByRole("heading", { name: translate(language, "SDrive is locked") });
  expect(screen.getByLabelText(translate(language, "Application PIN"))).toBeDefined();
  await waitFor(() => expect(document.documentElement.lang).toBe(language));
  expect(screen.queryByText("private management")).toBeNull();
});

test("lock event immediately removes management controls and discards an older status response", async () => {
  let changed!: (locked:boolean) => void;
  vi.mocked(api.onAccessChanged).mockImplementation((callback) => { changed = callback; return () => {}; });
  render(<LanguageProvider><AccessBoundary>{() => <label>Private value<input defaultValue="synthetic secret" /></label>}</AccessBoundary></LanguageProvider>);
  await screen.findByLabelText("Private value");
  let resolve!: (next:AccessStatus) => void;
  vi.mocked(api.accessStatus).mockReturnValueOnce(new Promise((done) => { resolve = done; }));
  act(() => changed(false));
  vi.mocked(api.accessStatus).mockResolvedValue({ ...status,locked:true });
  act(() => changed(true));
  expect(screen.queryByLabelText("Private value")).toBeNull();
  await screen.findByRole("heading",{name:"SDrive is locked"});
  await act(async () => { resolve(status); });
  expect(screen.queryByLabelText("Private value")).toBeNull();
  fireEvent.change(screen.getByLabelText("Application PIN"),{target:{value:"123456"}});
  vi.mocked(api.unlockAccess).mockResolvedValue();
  vi.mocked(api.accessStatus).mockResolvedValue(status);
  fireEvent.click(screen.getByRole("button",{name:"Unlock"}));
  await screen.findByLabelText("Private value");
});

test("access settings validate confirmation and retain the one-time recovery code until acknowledged", async () => {
  const close = vi.fn();
  vi.mocked(api.accessStatus).mockResolvedValue({...status,enabled:false});
  vi.mocked(api.configureAccess).mockResolvedValue("SYNTHETIC-RECOVERY-CODE");
  render(<LanguageProvider><AccessSettings onClose={close} /></LanguageProvider>);
  await waitFor(() => expect(screen.getByRole("button",{name:"Save"}).hasAttribute("disabled")).toBe(false));
  fireEvent.change(screen.getByLabelText("New PIN"),{target:{value:"123456"}});
  fireEvent.change(screen.getByLabelText("Confirm new PIN"),{target:{value:"654321"}});
  fireEvent.submit((screen.getByRole("button",{name:"Save"}) as HTMLButtonElement).form!);
  expect(screen.getByRole("alert").textContent).toBe("PIN confirmation does not match.");
  expect(api.configureAccess).not.toHaveBeenCalled();
  fireEvent.change(screen.getByLabelText("Confirm new PIN"),{target:{value:"123456"}});
  fireEvent.submit((screen.getByRole("button",{name:"Save"}) as HTMLButtonElement).form!);
  await screen.findByText("SYNTHETIC-RECOVERY-CODE");
  fireEvent(screen.getByRole("dialog"),new Event("cancel",{cancelable:true}));
  expect(close).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button",{name:"I saved the recovery code"}));
  expect(close).toHaveBeenCalledOnce();
});

test("recovery does not unlock until the replacement PIN is used and Portuguese remains available", async () => {
  vi.mocked(api.accessStatus).mockResolvedValue({...status,locked:true,language:"pt-BR"});
  vi.mocked(api.recoverAccess).mockResolvedValue("NOVO-CODIGO-SINTETICO");
  render(<LanguageProvider><AccessBoundary>{() => <p>private management</p>}</AccessBoundary></LanguageProvider>);
  await screen.findByRole("heading",{name:"O SDrive está bloqueado"});
  fireEvent.click(screen.getByRole("button",{name:"Esqueceu o PIN?"}));
  fireEvent.change(screen.getByLabelText("Código de recuperação"),{target:{value:"OLD-SYNTHETIC"}});
  fireEvent.change(screen.getByLabelText("Novo PIN"),{target:{value:"654321"}});
  fireEvent.submit(screen.getByRole("button",{name:"Redefinir PIN"}).closest("form")!);
  await screen.findByText("NOVO-CODIGO-SINTETICO");
  expect(api.unlockAccess).not.toHaveBeenCalled();
  expect(screen.queryByText("private management")).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"Guardei o código de recuperação"}));
  expect(screen.getByLabelText("PIN do aplicativo")).toHaveProperty("value","");
});

test("file access reports unknown separately from an empty Windows result and refreshes only the selected file", async () => {
  vi.mocked(api.inspectFileAccess).mockResolvedValueOnce({state:"unknown",processes:[],checkedAt:"2026-01-01T00:00:00Z",path:"file.txt"}).mockResolvedValueOnce({state:"reported",processes:[],checkedAt:"2026-01-01T00:00:01Z",path:"file.txt"});
  render(<StrictMode><LanguageProvider><FileAccessInspection connectionID="drive" disabled={false} onRun={async (work) => { await work(); }} /></LanguageProvider></StrictMode>);
  fireEvent.click(screen.getByRole("button",{name:"Select mounted file"}));
  await screen.findByText("File access information is unknown. This does not mean the file is unused.");
  fireEvent.click(screen.getByRole("button",{name:"Refresh"}));
  await screen.findByText("Windows reported no applications for this file. Other access may still exist.");
  expect(api.inspectFileAccess).toHaveBeenLastCalledWith("drive","file.txt");
});

test("Linux keeps manual PIN protection and disables global inactivity automation", async () => {
 vi.mocked(api.accessStatus).mockResolvedValue({ ...status, platform: "linux", globalIdle: false, policy: { idleMinutes: 0, disconnectIdle: false, reconnect: false } });
 render(<LanguageProvider><AccessSettings onClose={() => {}} /></LanguageProvider>);
 await screen.findByText("Global inactivity automation is unavailable on Linux. You can lock SDrive manually.");
 expect(screen.getByLabelText("Lock after computer inactivity (minutes; 0 disables)").closest("fieldset")?.disabled).toBe(true);
 expect((screen.getByLabelText("Current PIN") as HTMLInputElement).disabled).toBe(false);
});
