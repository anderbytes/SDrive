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
  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);
  return (
    <Locale.Provider value={{ language, setLanguage }}>
      {children}
    </Locale.Provider>
  );
}

export function useLanguage() {
  const { language, setLanguage } = useContext(Locale);
  return {
    language,
    setLanguage,
    t: (text: string | LocalizedMessage, values?: Record<string, string | number>) =>
      translate(language, text, values),
  };
}
