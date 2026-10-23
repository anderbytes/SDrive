import { expect, test } from "vitest";
import { Call } from "@wailsio/runtime";
import { validationError } from "./api";

test("field errors use Wails RuntimeError.cause without parsing error text", () => {
  const cause = { field: "port", message: "invalid port" };
  const error = new Call.RuntimeError("invalid port", { cause });
  expect(validationError(error)).toEqual(cause);
  expect(error.message).toBe("invalid port");
  for (const invalid of [undefined, new Error("invalid port"), { cause },
    new Call.RuntimeError("invalid port", { cause: { field: 42, message: "invalid port" } }),
    new Call.RuntimeError("invalid port", { cause: { field: "port", message: null } })]) {
    expect(validationError(invalid)).toBeUndefined();
  }
});
