import { Call, Events } from "@wailsio/runtime";

export interface AboutInfo {
  appVersion: string;
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
  advanced?: boolean;
  hint?: string;
  file?: boolean;
}
export interface Provider {
  id: string;
  name: string;
  backend: string;
  experimental: boolean;
  fields: Field[];
  oauth?: boolean;
  available?: boolean;
  notice?: string;
}
export interface Connection {
  id: string;
  remote: string;
  name: string;
  provider: string;
  path: string;
  letter: string;
  readOnly: boolean;
  autoMount: boolean;
  recovery: boolean;
  targetHash: string;
}
export interface ConnectionView extends Connection {
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
  error: string;
}
export interface Settings {
  language?: "en" | "pt-BR";
  autoStart: boolean;
  cacheGiB: number;
}
export interface Snapshot {
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
  Unsupported?: boolean;
}
export interface Session {
  id: string;
  busy: boolean;
  complete: boolean;
  question: Question | null;
  error: string;
  authenticated?: boolean;
  connectionID?: string;
}
export interface ConfigInput {
  connection: Connection;
  values: Record<string, string>;
  allowInsecure: boolean;
  transport: string;
  hostKey: string;
  guided?: boolean;
}

export interface FieldError {
  field: string;
  message: string;
}

export function validationError(error: unknown): FieldError | undefined {
  const cause = error instanceof Error ? error.cause : undefined;
  if (cause && typeof cause === "object" && "field" in cause && "message" in cause &&
      typeof cause.field === "string" && typeof cause.message === "string") {
    return { field: cause.field, message: cause.message };
  }
}

function invoke<T>(method: string, ...args: unknown[]): Promise<T> {
  return Call.ByName(`sdrive/internal/core.Service.${method}`, ...args);
}
export const api = {
  about: () => invoke<AboutInfo>("About"),
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
  confirm: (id: string, path: string, letter: string) =>
    invoke<string>("ConfirmConfig", id, path, letter),
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
  open: (id: string) => invoke<void>("OpenExplorer", id),
  prerequisite: (name: string) => invoke<void>("Prerequisite", name),
  probe: (host: string, port: string) =>
    invoke<{ key: string; fingerprint: string }>("ProbeSSH", host, port),
  exit: () => invoke<void>("Exit"),
  restart: () => invoke<void>("RestartEngine"),
  restoreSettings: () => invoke<void>("RestoreSettings"),
  verifyRecovery: () => invoke<void>("VerifyRecovery"),
  retryCleanup: () => invoke<void>("RetryCleanup"),
};
