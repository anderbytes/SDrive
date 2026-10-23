import { formatMessage, type LocalizedMessage } from "./messages";
import {
  createContext,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from "react";
import portuguese from "../../internal/i18n/pt-BR.json";

export type Language = "en" | "pt-BR";
const translations: Record<string, string> = portuguese;
const Locale = createContext({
  language: "en" as Language,
  setLanguage: (_: Language) => {},
});

const escape = (text: string) => text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
const formats = Object.entries(translations)
  .filter(([key]) => /%[sdwv]/.test(key))
  .sort(([a], [b]) => {
    const difference = b.replace(/%[sdwv]/g, "").length - a.replace(/%[sdwv]/g, "").length;
    return difference || (a < b ? -1 : a > b ? 1 : 0);
  })
  .map(([key, value]) => ({
    pattern: new RegExp(
      `^${key
        .split(/%[sdwv]/)
        .map(escape)
        .join("([\\s\\S]*?)")}$`,
    ),
    value,
  }));

export function translate(
  language: Language,
  text: string | LocalizedMessage,
  values: Record<string, string | number> = {},
): string {
  if (typeof text !== "string") return formatMessage(language, text);
  let result = text;
  if (language === "pt-BR") {
    result = Object.hasOwn(translations, text) ? translations[text] : text;
    if (!Object.hasOwn(translations, text)) {
      for (const { pattern, value } of formats) {
        const match = text.match(pattern);
        if (match) {
          let index = 0;
          result = value.replace(/%[sdwv]/g, () => match[++index]);
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
