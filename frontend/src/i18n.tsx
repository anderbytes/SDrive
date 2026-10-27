import { formatMessage, type LocalizedMessage } from "./messages";
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import portuguese from "../../internal/i18n/pt-BR.json";
import spanish from "../../internal/i18n/es.json";
import french from "../../internal/i18n/fr.json";
import german from "../../internal/i18n/de.json";
import italian from "../../internal/i18n/it.json";
import chinese from "../../internal/i18n/zh-CN.json";

export const languages = {
  en: "English", "pt-BR": "Português (Brasil)", es: "Español",
  fr: "Français", de: "Deutsch", it: "Italiano", "zh-CN": "简体中文",
} as const;
export type Language = keyof typeof languages;
export function isLanguage(value: string): value is Language {
  return Object.hasOwn(languages, value);
}
const catalogs: Partial<Record<Language, Record<string, string>>> = {
  "pt-BR": portuguese, es: spanish, fr: french, de: german, it: italian, "zh-CN": chinese,
};
const Locale = createContext({
  platform: "windows",
  setPlatform: (_: string) => {},
  language: "en" as Language,
  setLanguage: (_: Language) => {},
});

const escape = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const formats = Object.entries(portuguese)
  .filter(([key]) => /%[sdwv]/.test(key))
  .sort(([a], [b]) => {
    const difference = b.replace(/%[sdwv]/g, "").length - a.replace(/%[sdwv]/g, "").length;
    return difference || (a < b ? -1 : a > b ? 1 : 0);
  })
  .map(([key]) => ({
    pattern: new RegExp(
      `^${key
        .split(/%[sdwv]/)
        .map(escape)
        .join("([\\s\\S]*?)")}$`,
    ),
    key,
  }));

export function translate(
  language: Language,
  text: string | LocalizedMessage,
  values: Record<string, string | number> = {},
): string {
  if (typeof text !== "string") return formatMessage(language, text);
  let result = text;
  const translations = Object.hasOwn(catalogs, language) ? catalogs[language] : undefined;
  if (translations) {
    result = Object.hasOwn(translations, text) ? translations[text] : text;
    if (!Object.hasOwn(translations, text)) {
      for (const { pattern, key } of formats) {
        const match = text.match(pattern);
        if (match && Object.hasOwn(translations, key)) {
          let index = 0;
          result = translations[key].replace(/%[sdwv]/g, () => match[++index]);
          break;
        }
      }
    }
  }
  return result.replace(/\{(\w+)\}/g, (placeholder, key: string) =>
    String(values[key] ?? placeholder),
  );
}

export function LanguageProvider({ children }: { children: ReactNode }) {
  const [language, setLanguage] = useState<Language>("en");
  const [platform, setPlatform] = useState("windows");
  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);
  return (
    <Locale.Provider value={{ language, setLanguage, platform, setPlatform }}>
      {children}
    </Locale.Provider>
  );
}

export function useLanguage() {
  const { language, setLanguage, platform, setPlatform } = useContext(Locale);
  return {
    language,
    setLanguage,
    platform,
    setPlatform,
    t: (text: string | LocalizedMessage, values?: Record<string, string | number>) =>
      translate(language, platform === "linux" && typeof text === "string" ? linuxLabels[text] || text : text, values),
  };
}

const linuxLabels: Record<string, string> = {
  "Run the SDrive installer again to prepare drive mounting.": "Install FUSE3 and ensure your user can access /dev/fuse. See the Linux installation guide.",
  "Your cloud files, right in Explorer.": "Your cloud files, right in your file manager.",
  "Open in Explorer": "Open in file manager",
  "Open Explorer after connecting": "Open file manager after connecting",
  "Start with Windows, in the tray": "Start when signing in",
  "Windows mount destination": "Mount destination",
  "System follows your Windows theme. Save to keep the preview; Cancel restores the saved theme.": "System follows your desktop theme. Save to keep the preview; Cancel restores the saved theme.",
  "Connect an account or server and choose how it appears in Windows.": "Connect an account or server and choose a mount folder.",
  "18 connection types · Credentials protected by Windows": "18 connection types · Credentials protected by your desktop keyring",
  "Credentials are encrypted. The cache contains local files accessible to your Windows user.": "Credentials are encrypted. The cache contains local files accessible to your user.",
  "The lock protects SDrive management controls. Drives that remain connected are still accessible in Explorer.": "The lock protects SDrive management controls. Mounted folders remain accessible to your applications.",
  "Virtual capacity in Explorer (GiB)": "Virtual capacity in file manager (GiB)",
  "Refresh files in Explorer": "Refresh files in file manager",
  "Directory metadata refreshed. Reopen the folder in Explorer to see remote changes.": "Directory metadata refreshed. Reopen the folder in your file manager to see remote changes.",
  "Folder mounts require local-disk mode. SDrive runs in your Windows session; elevated applications may see different drives. Local-disk access is restricted to your user.": "Mounted folders are restricted to your user. FUSE must be available in your desktop session.",
  "Installation needs repair": "Required components unavailable",
  "Run the SDrive installer again. It prepares the required components and may request administrator permission.": "Install FUSE3 and ensure your user can access /dev/fuse. See the Linux installation guide.",
  "Windows startup, proxy credentials and cache location stay local.": "Sign-in startup, proxy credentials and cache location stay local.",
  "When unchecked, names starting with a dot are marked hidden. Explorer decides whether to display them. Applies on the next connection.": "Your file manager controls whether names starting with a dot are displayed.",
  "External changes appear on the next directory access after this lifetime, or after Refresh files in Explorer. Some services detect changes earlier. This is separate from SDrive status updates and applies on the next connection.": "External changes appear after this lifetime or after refreshing directory metadata. Some services detect changes earlier. Applies on the next connection."
};
