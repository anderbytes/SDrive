import { expect, test } from "vitest";
import portuguese from "../../internal/i18n/pt-BR.json";
import { isLanguage, languages, translate, type Language } from "./i18n";
import structured from "../../internal/i18n/messages.json";
import spanish from "../../internal/i18n/es.json";
import french from "../../internal/i18n/fr.json";
import german from "../../internal/i18n/de.json";
import italian from "../../internal/i18n/it.json";
import chinese from "../../internal/i18n/zh-CN.json";

test("English defaults, Portuguese translations and unknown text fallback", () => {
  expect(translate("en", "My drives")).toBe("My drives");
  expect(translate("pt-BR", "My drives")).toBe("Minhas unidades");
  expect(translate("pt-BR", "Unknown provider message")).toBe(
    "Unknown provider message",
  );
  expect(translate("pt-BR", "constructor")).toBe("constructor");
  expect(translate("pt-BR", "Engine stopped (exit code 17; last signal: network). Restart SDrive to recover your drives.")).toBe(
    "O mecanismo parou (código de saída 17; último sinal: network). Reinicie o SDrive para recuperar suas unidades.",
  );
  expect(translate("pt-BR", "More actions for {name}", { name: "Root" })).toBe(
    "Mais ações para Root",
  );
  expect(translate("pt-BR", "letter S: is already in use")).toBe(
    "letra S: já está em uso",
  );
  expect(
    translate("pt-BR", "recover connection Root before exiting safely"),
  ).toBe("recupere a conexão Root antes de sair com segurança");
});

test("every translation preserves named and Go format placeholders", () => {
  const placeholders = (value: string) =>
    (value.match(/\{\w+\}|%[sdwv]/g) || []).sort();
  const catalogs = { "pt-BR": portuguese, es: spanish, fr: french, de: german, it: italian, "zh-CN": chinese };
  for (const [language, catalog] of Object.entries(catalogs)) {
    expect(Object.keys(catalog).sort(), language).toEqual(Object.keys(portuguese).sort());
    for (const [source, translated] of Object.entries(catalog)) {
      expect(translated.trim(), `${language}: ${source}`).not.toBe("");
      expect(placeholders(translated), `${language}: ${source}`).toEqual(placeholders(source));
      expect(translated.match(/%[sdwv]/g), `${language}: ${source}`).toEqual(source.match(/%[sdwv]/g));
      expect(translate(language as Language, source), `${language}: ${source}`).toBe(translated);
    }
  }
});

test.each(Object.keys(languages) as Language[])("%s supports static, legacy and structured messages with opaque parameters", (language) => {
  expect(isLanguage(language)).toBe(true);
  expect(isLanguage("constructor")).toBe(false);
  expect(isLanguage("unknown")).toBe(false);
  expect(translate("constructor" as Language, "name")).toBe("name");
  expect(translate("unknown" as Language, "My drives")).toBe("My drives");
  expect(translate(language, "Unknown provider message")).toBe("Unknown provider message");
  expect(translate(language, "constructor")).toBe("constructor");
  expect(translate(language, "Open SDrive")).not.toBe("");
  const template = "Engine stopped (exit code %d; last signal: %s). Restart SDrive to recover your drives.";
  const source = template.replace("%d", "17").replace("%s", "network");
  expect(translate(language, source)).toBe(translate(language, template).replace("%d", "17").replace("%s", "network"));
  for (const [code, entry] of Object.entries(structured)) {
    const params = Object.fromEntries(Array.from(entry.en.matchAll(/\{(\w+)\}/g), ([, name]) => [name, "Café {letter} %s 😀"]));
    const expected = entry[language].replace(/\{\w+\}/g, () => "Café {letter} %s 😀");
    expect(translate(language, { code, params, message: "English fallback" })).toBe(expected);
    expect(translate(language, { code, params: {}, message: "English fallback" })).toBe("English fallback");
  }
});
