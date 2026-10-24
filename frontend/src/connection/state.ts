import type { Connection, MountTarget, Session } from "../api";

type Saved = { id: string; target: MountTarget };
export type WizardState =
  | { step: "service" | "access" | "edit" }
  | { step: "auth"; sessionID: string; session?: Session }
  | { step: "folder"; sessionID: string; saved?: Saved }
  | { step: "complete"; sessionID: string; saved: Saved };

export type WizardEvent =
  | { type: "service" | "access" }
  | { type: "start"; sessionID: string }
  | { type: "status"; sessionID: string; session: Session }
  | { type: "answering"; sessionID: string }
  | { type: "saved"; id: string; target: MountTarget }
  | { type: "connected" };

export function initialWizardState(initial: Connection, auth: boolean): WizardState {
  return { step: !auth ? "edit" : initial.id ? "access" : "service" };
}

export function wizardReducer(state: WizardState, event: WizardEvent): WizardState {
  switch (event.type) {
    case "service":
      return state.step === "access" ? { step: "service" } : state;
    case "access":
      return state.step === "service" ? { step: "access" } : state;
    case "start":
      return state.step === "access" && event.sessionID
        ? { step: "auth", sessionID: event.sessionID } : state;
    case "status":
      if (state.step !== "auth" || state.sessionID !== event.sessionID) return state;
      return event.session.authenticated && !event.session.complete
        ? { step: "folder", sessionID: state.sessionID }
        : { ...state, session: event.session };
    case "answering":
      return state.step === "auth" && state.sessionID === event.sessionID && state.session
        ? { ...state, session: { ...state.session, busy: true } } : state;
    case "saved":
      return state.step === "folder" && event.id
        ? { ...state, saved: { id: event.id, target: event.target } } : state;
    case "connected":
      return state.step === "folder" && state.saved
        ? { ...state, step: "complete", saved: state.saved } : state;
  }
}
