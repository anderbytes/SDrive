import catalog from "../../internal/i18n/messages.json";

export interface LocalizedMessage {
  code: string;
  params: Record<string, string>;
  message: string;
}

export function localizedMessage(value: unknown): LocalizedMessage | undefined {
  if (!value || typeof value !== "object") return;
  const candidate = value as Partial<LocalizedMessage>;
  if (typeof candidate.code === "string" && typeof candidate.message === "string" &&
      candidate.params && typeof candidate.params === "object" && !Array.isArray(candidate.params) &&
      Object.values(candidate.params).every((value) => typeof value === "string"))
    return candidate as LocalizedMessage;
}

export function formatMessage(language: string, message: LocalizedMessage): string {
  if (!localizedMessage(message) || !Object.hasOwn(catalog, message.code)) return message.message;
  const entry = catalog[message.code as keyof typeof catalog];
  const template = Object.hasOwn(entry, language) ? entry[language as keyof typeof entry] : entry.en;
  const tokens = Array.from(template.matchAll(/\{(\w+)\}/g));
  if (tokens.some(([, key]) => !Object.hasOwn(message.params, key))) return message.message;
  return template.replace(/\{(\w+)\}/g, (_, key: string) => message.params[key]);
}

export function errorMessage(error: unknown): string | LocalizedMessage {
  const cause = error instanceof Error ? error.cause : undefined;
  const structured = localizedMessage(cause);
  if (structured) return structured;
  if (cause && typeof cause === "object" && "code" in cause && "message" in cause && typeof cause.message === "string")
    return { code: "", params: {}, message: cause.message };
  return error instanceof Error ? error.message : String(error);
}
