import { afterEach, expect, test } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { LanguageProvider, useLanguage } from "./i18n";
import { TransportCapability, TransportStatus } from "./TransportSecurity";
import type { ConnectionView, Provider } from "./api";
import fixtures from "./bridge-fixtures.json";

afterEach(cleanup);
const provider: Provider = { ...fixtures.samples.providerEmpty, fields: [], id: "webdav", transportEncryption: "optional" };
const connection: ConnectionView = { ...fixtures.samples.connectionUnknown, operation: "", mounted: true, transportSecurity: { state: "encrypted", protocol: "https", source: "configuration", checkedAt: null } };
function LanguageSwitch() {
  const { setLanguage } = useLanguage();
  return <button onClick={() => setLanguage("pt-BR")}>Português</button>;
}

test("selected capability updates with the provider and is localized", () => {
  const view = render(<TransportCapability provider={provider} />);
  expect(screen.getByRole("status").textContent).toContain("available to enable");
  view.rerender(<TransportCapability provider={{ ...provider, id: "sftp", transportEncryption: "required" }} />);
  expect(screen.getByRole("status").textContent).toContain("native and mandatory");
  expect(screen.getByText("SFTP always encrypts traffic using SSH.")).toBeTruthy();
  view.rerender(<LanguageProvider><LanguageSwitch /><TransportCapability provider={provider} /></LanguageProvider>);
  fireEvent.click(screen.getByText("Português"));
  expect(screen.getByRole("status").textContent).toContain("Criptografia do tráfego: habilitável");
});

test("connection status distinguishes effective settings, negotiation and missing evidence", () => {
  const view = render(<TransportStatus connection={connection} />);
  expect(screen.getByText("Traffic encrypted · HTTPS")).toBeTruthy();
  expect(screen.getByText(/effective connection settings/)).toBeTruthy();
  view.rerender(<TransportStatus connection={{ ...connection, transportSecurity: { state: "unencrypted", protocol: "smb", source: "negotiated", checkedAt: null } }} />);
  expect(screen.getByText("Traffic unencrypted · SMB")).toBeTruthy();
  expect(screen.getByText("Based on the negotiated SMB session and share.")).toBeTruthy();
  view.rerender(<TransportStatus connection={{ ...connection, transportSecurity: { state: "unknown", protocol: "", source: "", checkedAt: null } }} />);
  expect(screen.getByText("Traffic encryption could not be verified")).toBeTruthy();
  view.rerender(<TransportStatus connection={{ ...connection, mounted: false }} />);
  expect(view.container.textContent).toBe("");
});

test("SMB and MEGA security qualifications stay visible without opening help", () => {
  const view = render(<TransportCapability provider={{ ...provider, id: "smb" }} />);
  expect(screen.queryByRole("button")).toBeNull();
  expect(screen.getByText(/Signing alone does not encrypt/).tagName).toBe("SMALL");
  view.rerender(<TransportCapability provider={{ ...provider, id: "mega" }} />);
  expect(screen.queryByRole("button")).toBeNull();
  expect(screen.getByText(/over HTTP by default/).tagName).toBe("SMALL");
});

test("MEGA explains encrypted content separately from HTTP transport in both languages", () => {
  render(<LanguageProvider><LanguageSwitch /><TransportStatus connection={{ ...connection, provider: "mega", transportSecurity: { state: "unencrypted", protocol: "http", source: "configuration", checkedAt: "2026-10-25T12:00:00Z" } }} /></LanguageProvider>);
  expect(screen.getByText("Traffic unencrypted · HTTP")).toBeTruthy();
  expect(screen.getByText(/MEGA file content is already encrypted/)).toBeTruthy();
  fireEvent.click(screen.getByText("Português"));
  expect(screen.getByText("Tráfego sem criptografia · HTTP")).toBeTruthy();
  expect(screen.getByText(/O conteúdo dos arquivos MEGA já está criptografado/)).toBeTruthy();
});
