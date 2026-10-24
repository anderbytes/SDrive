import { expect, test } from "vitest";
import { wizardReducer, type WizardState } from "./state";
import fixtures from "../bridge-fixtures.json";

test("wizard requires a session, authentication, a saved connection and a successful mount in order", () => {
  let state: WizardState = { step: "access" };
  expect(wizardReducer(state, { type: "connected" })).toBe(state);
  expect(wizardReducer(state, { type: "start", sessionID: "" })).toBe(state);
  state = wizardReducer(state, { type: "start", sessionID: "session" });
  expect(state).toEqual({ step: "auth", sessionID: "session" });
  expect(wizardReducer(state, { type: "service" })).toBe(state);
  expect(wizardReducer(state, { type: "status", sessionID: "obsolete", session: fixtures.samples.sessionFilled })).toBe(state);
  state = wizardReducer(state, { type: "status", sessionID: "session", session: fixtures.samples.sessionFilled });
  expect(state).toEqual({ step: "folder", sessionID: "session" });
  expect(wizardReducer(state, { type: "connected" })).toBe(state);
  state = wizardReducer(state, { type: "saved", id: "saved", target: { letter: "S", folder: "", mode: "" } });
  expect(wizardReducer(state, { type: "status", sessionID: "session", session: fixtures.samples.sessionEmpty })).toBe(state);
  state = wizardReducer(state, { type: "saved", id: "saved", target: { letter: "T", folder: "", mode: "" } });
  state = wizardReducer(state, { type: "connected" });
  expect(state).toEqual({ step: "complete", sessionID: "session", saved: { id: "saved", target: { letter: "T", folder: "", mode: "" } } });
});
