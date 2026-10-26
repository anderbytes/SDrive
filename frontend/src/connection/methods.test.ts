import { expect, test } from "vitest";
import { methods, advancedDisclaimer } from "./methods";
import { translate } from "../i18n";
import type { Provider } from "../api";

test("setup categories classify authentication methods and reuse the GCS backend", () => {
  const provider = (id: string, oauth: boolean): Provider => ({ id, name: id, transportEncryption: "required", backend: id === "gcs" ? "google cloud storage" : id, oauth, available: true, experimental: false, fields: [] });
  const entries = methods([provider("drive", true), provider("ftp", false), provider("s3", false), provider("b2", false), provider("azureblob", false), provider("gcs", true), provider("yandex", true)]);
  expect(entries.filter((m) => !m.advanced).map((m) => m.key)).toEqual(["ftp", "s3", "b2", "azureblob", "gcs:serviceAccount"]);
  expect(entries.filter((m) => m.advanced).map((m) => m.key)).toEqual(["drive", "gcs:oauth", "yandex"]);
  const gcs = entries.filter((m) => m.provider.id === "gcs");
  expect(gcs[0].provider).toBe(gcs[1].provider);
});

test("Advanced disclaimer explains user-owned applications in both languages", () => {
  expect(translate("en", advancedDisclaimer)).toContain("does not maintain shared OAuth applications");
  expect(translate("pt-BR", advancedDisclaimer)).toContain("não mantém aplicativos OAuth compartilhados");
  expect(translate("pt-BR", advancedDisclaimer)).toContain("Client Secret");
});
