import { type Language } from "./i18n";
import { type Theme } from "./theme";
import { localizedMessage, type LocalizedMessage } from "./messages";
import { Call, Clipboard, Events } from "@wailsio/runtime";

export interface FileAccessView { path: string; state: string; checkedAt: string; processes: { pid: number; name: string; startedAt: string }[] }
export interface AccessPolicy { idleMinutes: number; disconnectIdle: boolean; reconnect: boolean }
export interface AccessStatus {
  platform?: string;
  globalIdle?: boolean; theme: string; setupPending: boolean; enabled: boolean; locked: boolean; language: string; policy: AccessPolicy; retryAt: string; error: string; warning: string }

export interface AboutInfo {
  platform?: string;
  osVersion?: string;
  appVersion: string;
  releaseDate?: string;
  rcloneVersion: string;
  windowsVersion: string;
  winFspVersion: string;
  webView2Version: string;
  architecture: string;
}

export interface Field {
  key: string;
  label: string;
  secret: boolean;
  required: boolean;
  default: string;
  choices?: string[];
  advanced: boolean;
  hint?: string;
  file: boolean;
}
export interface Provider {
	transportEncryption: string;
  id: string;
  name: string;
  backend: string;
  experimental: boolean;
  fields: Field[];
  oauth: boolean;
  available: boolean;
  notice?: string;
}
export interface ConnectionDetail { label: string; value: string }
export interface DriveAppearance { cardStyle?: string; iconImage?: string }
export interface Connection extends DriveAppearance {
  hiddenFileVisibility?: string;
  openExplorerAfterConnect?: boolean;
  id: string;
  remote: string;
  name: string;
  provider: string;
  path: string;
  letter: string;
  mountMode?: string;
  mountFolder?: string;
  directoryCacheSeconds?: number;
  virtualCapacityGiB?: number;
  readOnly: boolean;
  autoMount: boolean;
  recovery: boolean;
  targetHash: string;
  authMethod?: string;
  sessionOnly?: boolean;
  uploadLimit?: number;
  downloadLimit?: number;
  useProxy?: boolean;
  browser?: string;
  privateBrowser?: boolean;
  connectTimeout?: number;
  idleTimeout?: number;
  alternateURL?: string;
  resourceID?: string;
  primaryURL?: string;
  binding?: string;
}
export interface ConnectionView extends Connection {
  network: NetworkView;
  serverPath?: string;
	transportSecurity: TransportSecurityView;
  mounted: boolean;
  busy: boolean;
  operation: "" | "validating" | "mounting" | "checkingUploads" | "waitingUploads" | "unmounting";
  pending: number | null;
  uploads: { active: number; queued: number; cacheErrors: number } | null;
  cache: {
    checkedAt: string | null;
    pendingBytes: number | null;
    usedBytes: number | null;
    error: string;
  };
  capacity: { total: number | null; used: number | null; free: number | null; checkedAt: string | null; state: string };
  error: string;
  errorMessage?: LocalizedMessage;
}
export interface TransportSecurityView { state: string; protocol: string; source: string; checkedAt: string | null }
export interface NetworkView {
  state: string;
  partial: boolean;
  startedAt: string | null;
  checkedAt: string | null;
  sent: number | null;
  received: number | null;
  filesSent: number | null;
  filesReceived: number | null;
}
export interface Settings {
  driveStripeColorSource?: "theme" | "drive";
  theme: Theme;
	showServiceIcons: boolean;
  language: Language;
  autoStart: boolean;
  cacheGiB: number;
  cacheMaxAgeHours?: number;
  writeBackSeconds?: number;
}
export interface MountTarget { letter: string; folder: string; mode: string }
export const mountTarget = (c: Connection): MountTarget => ({ letter: c.letter, folder: c.mountFolder || "", mode: c.mountMode || "" });
export interface Snapshot {
  platform?: string;
  mountAvailable?: boolean;
  access: AccessStatus;
	importDrafts: ImportDraft[];
	desktopWarning: string;
  cacheLocation: string;
  freeBytes: number | null;
  providers: Provider[];
  connections: ConnectionView[];
  settings: Settings;
  letters: string[];
  winFsp: boolean;
  engineError: string;
  settingsError: string;
  backupAvailable: boolean;
  recoveryHold: boolean;
  maintenanceError: string;
  startupWarning: string;
}
export interface Question {
  Name: string;
  Help: string;
  DefaultStr: string;
  Type: string;
  IsPassword: boolean;
  Sensitive: boolean;
  Required: boolean;
  Exclusive: boolean;
  Examples: { Value: string; Help: string }[] | null;
  Unsupported: boolean;
}
export interface Session {
  id: string;
  busy: boolean;
  complete: boolean;
  question: Question | null;
  error: string;
  authenticated: boolean;
  connectionID: string;
}
export interface ConfigInput {
	importDraftID?: string;
  connection: Connection;
  values: Record<string, string>;
  allowInsecure: boolean;
  transport: string;
  hostKey: string;
  guided: boolean;
  forgetCredentials?: boolean;
}

export interface Browser { id: string; name: string }
export interface ProxyConfig { address: string; username: string; password: string; hasPassword: boolean }

export type PortableConnection = Omit<Connection, "id" | "remote" | "recovery" | "targetHash" | "resourceID" | "primaryURL" | "binding"> & { values: Record<string, string>; transport: string };
export type PortablePreferences = Omit<Settings, "autoStart">;
export interface PortableFile { format: string; version: number; connections: PortableConnection[]; preferences: PortablePreferences }
export interface ImportDraft { id: string; definition: PortableConnection }
export interface ImportPreview { id: string; document: PortableFile }

export interface FieldError {
  field: string;
  message: string;
  code?: string;
  params?: Record<string, string>;
}

export function validationError(error: unknown): FieldError | undefined {
  const cause = error instanceof Error ? error.cause : undefined;
  if (cause && typeof cause === "object" && "field" in cause && "message" in cause &&
      typeof cause.field === "string" && typeof cause.message === "string") {
    const metadata = localizedMessage(cause) || ("code" in cause ? { code: "", params: {} } : undefined);
    return { ...metadata, field: cause.field, message: cause.message };
  }
}

function invoke<T>(method: string, ...args: unknown[]): Promise<T> {
  return Call.ByName(`sdrive/internal/core.Service.${method}`, ...args);
}
export const api = {
  reorderConnections: (ids: string[]) => invoke<void>("ReorderConnections", ids),
  connectionDetails: (id: string) => invoke<ConnectionDetail[]>("ConnectionDetails", id),
  exportDiagnosticReport: (report: string) => invoke<boolean>("ExportDiagnosticReport", report),
  updateDriveAppearance: (id: string, appearance: DriveAppearance) => invoke<void>("UpdateDriveAppearance", id, appearance),
  inspectFileAccess: (id: string, path: string) => invoke<FileAccessView>("InspectFileAccess", id, path),
  accessStatus: () => invoke<AccessStatus>("AccessStatus"),
  configureAccess: (currentPIN: string, newPIN: string, policy: AccessPolicy) => invoke<string>("ConfigureAccess", currentPIN, newPIN, policy),
  disableAccess: (pin: string) => invoke<void>("DisableAccess", pin),
  lock: () => invoke<void>("Lock"),
  unlockAccess: (pin: string) => invoke<void>("UnlockAccess", pin),
  recoverAccess: (code: string, pin: string) => invoke<string>("RecoverAccess", code, pin),
  onAccessChanged: (callback: (locked: boolean) => void) => Events.On("access:changed", (event) => callback(event.data === true)),
	exportConfiguration: () => invoke<void>("ExportConfiguration"),
	previewImport: () => invoke<ImportPreview | null>("PreviewImport"),
	commitImport: (id: string, document: PortableFile, applyPreferences: boolean) => invoke<void>("CommitImport", id, document, applyPreferences),
	removeImportDraft: (id: string) => invoke<void>("RemoveImportDraft", id),
  selectDirectory: () => invoke<string>("SelectDirectory"),
  preferenceDefaults: () => invoke<Settings>("PreferenceDefaults"),
  restoreDefaultCacheLocation: () => invoke<string>("RestoreDefaultCacheLocation"),
  changeCacheLocation: (path: string) => invoke<void>("ChangeCacheLocation", path),
  refreshDirectoryCache: (id: string) => invoke<void>("RefreshDirectoryCache", id),
  browsers: () => invoke<Browser[]>("Browsers"),
  proxy: () => invoke<ProxyConfig>("ProxySettings"),
  saveProxy: (proxy: ProxyConfig) => invoke<void>("SaveProxy", proxy),
  about: () => invoke<AboutInfo>("About"),
  diagnostics: () => invoke<string>("Diagnostics"),
  copyDiagnostics: (report: string) => Clipboard.SetText(report),
  desktopVisible: () => invoke<boolean>("DesktopVisible"),
  onDesktopVisibility: (callback: (visible: boolean) => void) =>
    Events.On("desktop:visibility", (event) => { if (typeof event.data === "boolean") callback(event.data); }),
  snapshot: () => invoke<Snapshot>("Snapshot"),
  begin: (input: ConfigInput) => invoke<string>("BeginConfig", input),
  status: (id: string) => invoke<Session>("ConfigStatus", id),
  answer: (id: string, answer: string) =>
    invoke<void>("AnswerConfig", id, answer),
  end: (id: string) => invoke<void>("EndConfig", id),
  folders: (id: string, path: string) =>
    invoke<{ name: string; path: string }[]>("ListConfigFolders", id, path),
  confirm: (id: string, path: string, target: MountTarget) =>
    invoke<string>("ConfirmConfig", id, path, target),
  selectFile: (kind: string) => invoke<string>("SelectCredentialFile", kind),
  mount: (id: string) => invoke<void>("Mount", id),
  unmount: (id: string) => invoke<void>("Unmount", id),
  test: (id: string) => invoke<void>("TestConnection", id),
  remove: (id: string) => invoke<void>("Remove", id),
  update: (connection: Connection) =>
    invoke<void>("UpdateConnection", connection),
  fields: (id: string) =>
    invoke<{ values: Record<string, string>; transport: string }>(
      "ConnectionFields", id,
    ),
  settings: (settings: Settings) => invoke<void>("SaveSettings", settings),
  open: (id: string) => invoke<void>("OpenMount", id),
  prerequisite: (name: string) => invoke<void>("Prerequisite", name),
  probe: (host: string, port: string) =>
    invoke<{ key: string; fingerprint: string }>("ProbeSSH", host, port),
  exit: () => invoke<void>("Exit"),
  restart: () => invoke<void>("RestartEngine"),
  restoreSettings: () => invoke<void>("RestoreSettings"),
  verifyRecovery: () => invoke<void>("VerifyRecovery"),
  retryCleanup: () => invoke<void>("RetryCleanup"),
};
