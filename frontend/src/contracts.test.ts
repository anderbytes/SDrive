import { beforeEach, expect, expectTypeOf, test, vi } from "vitest";
import { Call } from "@wailsio/runtime";
import { api, type AccessStatus, type FileAccessView, type PortableConnection, type PortablePreferences, type PortableFile, type ImportDraft, type ImportPreview, type MountTarget, type Browser, type ProxyConfig, type AboutInfo, type ConfigInput, type Connection, type ConnectionView, type Field, type FieldError, type Provider, type Question, type Session, type Settings, type Snapshot } from "./api";
import fixtures from "./bridge-fixtures.json";

vi.mock("@wailsio/runtime", () => ({ Call: { ByName: vi.fn() } }));
beforeEach(() => vi.resetAllMocks());
const samples = fixtures.samples;
const filled = samples.snapshotFilled;
const connection = filled.connections[0];
const question = samples.sessionFilled.question;
type RequiredKeys<T> = { [K in keyof T]-?: {} extends Pick<T, K> ? never : K }[keyof T];

test("portable definitions and update status preserve the Go wire shape", () => {
  expectTypeOf<keyof AccessStatus>().toEqualTypeOf<keyof typeof samples.accessStatus>();
  expectTypeOf<keyof FileAccessView>().toEqualTypeOf<keyof typeof samples.fileAccess>();
  expectTypeOf<keyof PortableConnection>().toEqualTypeOf<keyof typeof samples.importDraft.definition>();
  expectTypeOf<keyof PortablePreferences>().toEqualTypeOf<keyof typeof samples.importPreview.document.preferences>();
  expectTypeOf<keyof PortableFile>().toEqualTypeOf<keyof typeof samples.importPreview.document>();
  expectTypeOf<keyof ImportDraft>().toEqualTypeOf<keyof typeof samples.importDraft>();
  expectTypeOf<keyof ImportPreview>().toEqualTypeOf<keyof typeof samples.importPreview>();
  expect(samples.importPreview.document.format).toBe("sdrive-portable");
});

test("Go wire samples satisfy the TypeScript bridge, including nested keys and nulls", () => {
  const settings: Settings = { ...filled.settings, theme: "system", language: "en", driveStripeColorSource: "theme" };
  const snapshot: Snapshot = { ...filled, settings, connections: [{ ...connection, operation: "mounting" }] };
  const empty: Snapshot = { ...samples.snapshotEmpty, settings };
  const unknown: ConnectionView = { ...samples.connectionUnknown, operation: "" };
  const session: Session = samples.sessionFilled;
  const emptySession: Session = samples.sessionEmpty;
  const input: ConfigInput = samples.configInput;
  const field: Field = filled.providers[0].fields[0];
  const provider: Provider = filled.providers[0];
  const about: AboutInfo = samples.about;
  const error: FieldError = samples.dynamicFieldError;
  expect([snapshot, empty, unknown, session, emptySession, input, field, provider, about, error]).toHaveLength(10);
  expectTypeOf<keyof Snapshot>().toEqualTypeOf<keyof typeof filled>();
  expectTypeOf<keyof Connection>().toEqualTypeOf<keyof typeof input.connection>();
  expectTypeOf<keyof ConnectionView>().toEqualTypeOf<keyof typeof connection>();
  expectTypeOf<keyof Provider>().toEqualTypeOf<keyof typeof filled.providers[0]>();
  expectTypeOf<keyof Field>().toEqualTypeOf<keyof typeof filled.providers[0]["fields"][0]>();
  expectTypeOf<keyof Session>().toEqualTypeOf<keyof typeof samples.sessionFilled>();
  expectTypeOf<keyof Question>().toEqualTypeOf<keyof typeof question>();
  expectTypeOf<keyof Settings>().toEqualTypeOf<keyof typeof filled.settings>();
  expectTypeOf<keyof ConfigInput>().toEqualTypeOf<keyof typeof samples.configInput>();
  expectTypeOf<keyof AboutInfo>().toEqualTypeOf<keyof typeof samples.about>();
  expectTypeOf<keyof FieldError>().toEqualTypeOf<keyof typeof samples.dynamicFieldError>();
  expectTypeOf<keyof ConnectionView["cache"]>().toEqualTypeOf<keyof typeof connection.cache>();
  expectTypeOf<keyof ConnectionView["capacity"]>().toEqualTypeOf<keyof typeof connection.capacity>();
  expectTypeOf<keyof ConnectionView["network"]>().toEqualTypeOf<keyof typeof connection.network>();
  expectTypeOf<ConnectionView["network"]["sent"]>().toEqualTypeOf<number | null>();
  expectTypeOf<keyof ConnectionView["transportSecurity"]>().toEqualTypeOf<keyof typeof connection.transportSecurity>();
  expectTypeOf<keyof MountTarget>().toEqualTypeOf<keyof typeof samples.mountTarget>();
  expectTypeOf<keyof NonNullable<ConnectionView["uploads"]>>().toEqualTypeOf<keyof typeof connection.uploads>();
  expectTypeOf<ConnectionView["pending"]>().toEqualTypeOf<number | null>();
  expectTypeOf<Snapshot["freeBytes"]>().toEqualTypeOf<number | null>();
  expectTypeOf<ConnectionView["cache"]["checkedAt"]>().toEqualTypeOf<string | null>();
  expectTypeOf<ConnectionView["cache"]["pendingBytes"]>().toEqualTypeOf<number | null>();
  expectTypeOf<ConnectionView["cache"]["usedBytes"]>().toEqualTypeOf<number | null>();
  expectTypeOf<Settings["language"]>().toEqualTypeOf<"en" | "pt-BR" | "es" | "fr" | "de" | "it" | "zh-CN">();
  expectTypeOf<Session["authenticated"]>().toEqualTypeOf<boolean>();
  expectTypeOf<Session["connectionID"]>().toEqualTypeOf<string>();
  expectTypeOf<RequiredKeys<Provider>>().toEqualTypeOf<keyof typeof samples.providerEmpty>();
  expectTypeOf<RequiredKeys<Field>>().toEqualTypeOf<keyof typeof samples.fieldEmpty>();
  expectTypeOf<RequiredKeys<ConnectionView>>().toEqualTypeOf<keyof typeof samples.connectionUnknown>();
  expectTypeOf<RequiredKeys<FieldError>>().toEqualTypeOf<keyof typeof samples.fieldError>();
  expectTypeOf<Question["Examples"]>().toEqualTypeOf<{ Value: string; Help: string }[] | null>();
  expect(connection.operation).toBe(snapshot.connections[0].operation);
  expect(unknown.pending).toBeNull();
  expect(empty.connections).toEqual([]);
});

type BridgeKey = Exclude<keyof typeof api, "copyDiagnostics" | "onDesktopVisibility" | "onAccessChanged">;
type Calls = { [K in BridgeKey]: Parameters<(typeof api)[K]> };
type Results = {
  updateDriveAppearance: void;
  inspectFileAccess: FileAccessView; accessStatus: AccessStatus; configureAccess: string; disableAccess: void; lock: void; unlockAccess: void; recoverAccess: string;
  exportConfiguration: void; previewImport: ImportPreview | null; commitImport: void; removeImportDraft: void;
  preferenceDefaults: Settings; restoreDefaultCacheLocation: string;
  selectDirectory: string; changeCacheLocation: void; refreshDirectoryCache: void;
  browsers: Browser[]; proxy: ProxyConfig; saveProxy: void;
  about: AboutInfo; diagnostics: string; desktopVisible: boolean; snapshot: Snapshot;
  begin: string; status: Session; answer: void; end: void; folders: typeof samples.folders;
  confirm: string; selectFile: string; mount: void; unmount: void; test: void; remove: void;
  update: void; fields: { values: Record<string, string>; transport: string }; settings: void;
  open: void; prerequisite: void; probe: typeof samples.identity; exit: void; restart: void;
  restoreSettings: void; verifyRecovery: void; retryCleanup: void;
};
const calls = {
  updateDriveAppearance: ["connection", { cardStyle: "blue", iconImage: "" }],
  inspectFileAccess: ["connection", "file.txt"], accessStatus: [], configureAccess: ["", "123456", { idleMinutes: 15, disconnectIdle: false, reconnect: false }], disableAccess: ["123456"], lock: [], unlockAccess: ["123456"], recoverAccess: ["synthetic", "123456"],
  exportConfiguration: [], previewImport: [], commitImport: ["preview", { ...samples.importPreview.document, preferences: { ...samples.importPreview.document.preferences, theme: "system", language: "en", driveStripeColorSource: "theme" } }, false], removeImportDraft: ["draft"],
  preferenceDefaults: [], restoreDefaultCacheLocation: [],
  selectDirectory: [], changeCacheLocation: ["C:\\Synthetic\\Cache"], refreshDirectoryCache: ["connection"],
  browsers: [], proxy: [], saveProxy: [{ address: "", username: "", password: "", hasPassword: false }],
  about: [], diagnostics: [], desktopVisible: [], snapshot: [],
  begin: [samples.configInput], status: ["session"], answer: ["session", "answer"], end: ["session"],
  folders: ["session", "folder"], confirm: ["session", "folder", { letter: "S", folder: "", mode: "" }], selectFile: ["service_account_file"],
  mount: ["connection"], unmount: ["connection"], test: ["connection"], remove: ["connection"],
  update: [samples.configInput.connection], fields: ["connection"],
  settings: [{ ...filled.settings, theme: "system", language: "en", driveStripeColorSource: "theme" }], open: ["connection"], prerequisite: ["winfsp"],
  probe: ["example.test", "22"], exit: [], restart: [], restoreSettings: [], verifyRecovery: [], retryCleanup: [],
} satisfies Calls;

test("every client method targets the checked Go method and forwards its arguments and result", async () => {
  expectTypeOf<{ [K in BridgeKey]: Awaited<ReturnType<(typeof api)[K]>> }>().toEqualTypeOf<Results>();
  expect(Object.keys(calls).sort()).toEqual(Object.keys(fixtures.methods).sort());
  const result = { synthetic: true };
  vi.mocked(Call.ByName).mockResolvedValue(result);
  for (const key of Object.keys(calls) as BridgeKey[]) {
    const contract = fixtures.methods[key];
    const args = calls[key];
    expect(args).toHaveLength(contract.args.length);
    const invoke = api[key] as (...args: unknown[]) => Promise<unknown>;
    expect(await invoke(...args)).toBe(result);
    expect(Call.ByName).toHaveBeenLastCalledWith(`sdrive/internal/core.Service.${contract.name}`, ...args);
  }
  expectTypeOf<Awaited<ReturnType<typeof api.folders>>>().toEqualTypeOf<typeof samples.folders>();
  expectTypeOf<keyof Awaited<ReturnType<typeof api.fields>>>().toEqualTypeOf<keyof typeof samples.fields>();
  const fields: Awaited<ReturnType<typeof api.fields>> = samples.fields;
  expect(fields.values).toEqual({});
  expectTypeOf<Awaited<ReturnType<typeof api.probe>>>().toEqualTypeOf<typeof samples.identity>();
});
