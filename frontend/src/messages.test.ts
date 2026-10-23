import { expect, test } from "vitest";
import { Call } from "@wailsio/runtime";
import { errorMessage, formatMessage, localizedMessage, type LocalizedMessage } from "./messages";
import { translate } from "./i18n";
import { validationError } from "./api";
import fixtures from "./bridge-fixtures.json";

test("structured localization uses codes, preserves opaque values and falls back unchanged", () => {
  const message = { code: "recovery.required_before_exit", params: { name: "Café {letter} %s 😀" }, message: "New English wording" };
  expect(translate("pt-BR", message)).toBe("recupere a conexão Café {letter} %s 😀 antes de sair com segurança");
  expect(translate("en", message)).toBe("recover connection Café {letter} %s 😀 before exiting safely");
  expect(formatMessage("unknown", message)).toBe(translate("en", message));
  expect(translate("pt-BR", { ...message, code: "unknown" })).toBe("New English wording");
  expect(translate("pt-BR", { ...message, params: {} })).toBe("New English wording");
  for (const params of [null, [], "text", { name: 42 }])
    expect(localizedMessage({ ...message, params })).toBeUndefined();
  const malformed = { code: "validation.required", params: { label: 42 }, message: "fill in Server" };
  expect(formatMessage("pt-BR", malformed as unknown as LocalizedMessage)).toBe("fill in Server");
  const invalidError = new Call.RuntimeError(malformed.message, { cause: { ...malformed, field: "host" } });
  expect(translate("pt-BR", errorMessage(invalidError))).toBe("fill in Server");
  expect(translate("pt-BR", localizedMessage(validationError(invalidError))!)).toBe("fill in Server");
  const field = fixtures.samples.dynamicFieldError;
  const error = new Call.RuntimeError("Changed wrapper wording", { cause: field });
  expect(validationError(error)).toEqual(field);
  expect(translate("pt-BR", localizedMessage(validationError(error))!)).toBe("preencha Server");
});
