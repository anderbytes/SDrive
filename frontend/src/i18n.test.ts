import { expect, test } from "vitest";
import portuguese from "../../internal/i18n/pt-BR.json";
import { translate } from "./i18n";

test("English defaults, Portuguese translations and unknown text fallback", () => {
  expect(translate("en", "My drives")).toBe("My drives");
  expect(translate("pt-BR", "My drives")).toBe("Minhas unidades");
  expect(translate("pt-BR", "Unknown provider message")).toBe(
    "Unknown provider message",
  );
  expect(translate("pt-BR", "constructor")).toBe("constructor");
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
  for (const [source, translated] of Object.entries(portuguese)) {
    expect(translated.trim(), source).not.toBe("");
    expect(placeholders(translated), source).toEqual(placeholders(source));
  }
});
