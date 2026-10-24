import { errorMessage as message, localizedMessage, type LocalizedMessage } from "../messages";
import { driveNameError, trimDriveName } from "./name";
import { useEffect, useId, useState, useReducer } from "react";
import { api, validationError, type Browser, type Connection, type FieldError, type Snapshot } from "../api";
import { methods, methodKey } from "./methods";
import { useLanguage } from "../i18n";
import { useDialog } from "../useDialog";
import { initialWizardState, wizardReducer } from "./state";

export interface EditorProps {
  initial: Connection;
  auth: boolean;
  data: Snapshot;
  onRefresh: () => Promise<void>;
  onClose: () => void;
}

export function useConnectionWizard({
  initial,
  auth,
  data,
  onRefresh,
  onClose,
}: EditorProps) {
  const { t } = useLanguage();
  const serviceMethods = methods(data.providers);
  const firstBasic = serviceMethods.find((m) => !m.advanced);
  const [connection, setConnection] = useState(initial.id ? initial : { ...initial, provider: firstBasic?.provider.id || data.providers[0].id, authMethod: firstBasic?.authMethod || "oauth" });
  const [category, setCategory] = useState(initial.id && data.providers.find((p) => p.id === initial.provider)?.oauth && initial.authMethod !== "serviceAccount" ? "advanced" : "basic");
  const [browsers, setBrowsers] = useState<Browser[]>([{ id: "", name: "Default browser" }]);
  const [forgetCredentials, setForgetCredentials] = useState(false);
  const [values, setValues] = useState<Record<string, string>>({});
  useEffect(() => { let active = true; void api.browsers().then((value) => { if (active) setBrowsers(value); }).catch(() => {}); return () => { active = false; }; }, []);
  const [state, dispatch] = useReducer(wizardReducer, initialWizardState(initial, auth));
  const { step } = state;
  const sessionID = "sessionID" in state ? state.sessionID : "";
  const session = state.step === "auth" ? state.session : undefined;
  const saved = "saved" in state ? state.saved : undefined;
  const savedID = saved?.id || "";
  const savedLetter = saved?.letter || "";
  const [transport, setTransport] = useState("explicit");
  const [insecure, setInsecure] = useState(false);
  const [identity, setIdentity] = useState<{
    key: string;
    fingerprint: string;
  }>();
  const [trusted, setTrusted] = useState(false);
  const [answer, setAnswer] = useState("");
  const [operation, setOperation] = useState<"idle" | "working" | "validating" | "mounting">("idle");
  const busy = operation !== "idle";
  const [accessLoad, setAccessLoad] = useState<"loading" | "ready" | "error">(initial.id && auth ? "loading" : "ready");
  const [folderLoad, setFolderLoad] = useState<"loading" | "ready" | "error">("ready");
  const loading = step === "folder" ? folderLoad === "loading" : accessLoad === "loading";
  const fieldsLoaded = accessLoad === "ready";
  const [error, setError] = useState<string | LocalizedMessage>("");
  const [folderError, setFolderError] = useState<string | LocalizedMessage>("");
  const [fieldError, setFieldError] = useState<FieldError>();
  const fieldID = useId();
  const [folderRequest, setFolderRequest] = useState(0);
  const [filter, setFilter] = useState("");
  const [folders, setFolders] = useState<{ name: string; path: string }[]>([]);
  const [path, setPath] = useState(initial.path);
  const [directPath, setDirectPath] = useState(initial.path);
  const p = data.providers.find((p) => p.id === connection.provider)!;
  const change = (patch: Partial<Connection>) => {
    setConnection((c) => ({ ...c, ...patch }));
    if (patch.name !== undefined) {
      const invalid = driveNameError(patch.name, auth && !initial.id ? p.name : "");
      setFieldError(invalid ? { field: "name", message: invalid } : undefined);
    } else if (fieldError && fieldError.field in patch) setFieldError(undefined);
  };
  const action = async (work: () => Promise<void>) => {
    setOperation("working");
    setError("");
    setFieldError(undefined);
    try {
      await work();
    } catch (e) {
      const invalid = validationError(e);
      if (invalid) setFieldError(invalid);
      else setError(message(e));
    } finally {
      setOperation("idle");
    }
  };
  useEffect(() => {
    let active = true;
    if (initial.id && auth)
      void api
        .fields(initial.id)
        .then((fields) => {
          if (active) {
            setValues(fields.values);
            if (initial.provider === "gcs") {
              const authMethod = fields.values.service_account_file ? "serviceAccount" : "oauth";
              setConnection((c) => ({ ...c, authMethod }));
              setCategory(authMethod === "oauth" ? "advanced" : "basic");
            }
            if (fields.transport) setTransport(fields.transport);
            setAccessLoad("ready");
          }
        })
        .catch((e) => {
          if (active) { setError(message(e)); setAccessLoad("error"); }
        });
    return () => {
      active = false;
    };
  }, [initial.id, auth]);
  useEffect(() => {
    if (!sessionID) return;
    let active = true;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const next = await api.status(sessionID);
        if (!active) return;
        dispatch({ type: "status", sessionID, session: next });
        if (!next.busy && next.question)
          setAnswer((previous) => previous || next.question?.DefaultStr || "");
      } catch (e) {
        if (active) setError(message(e));
      }
      if (active) timer = setTimeout(poll, 800);
    };
    void poll();
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [sessionID]);
  useEffect(() => {
    if (step !== "folder" || !sessionID || savedID) return;
    let active = true;
    setFolderLoad("loading");
    setFolderError("");
    setFolders([]);
    void api
      .folders(sessionID, path)
      .then((items) => {
        if (active) { setFolders(items); setFolderLoad("ready"); }
      })
      .catch((e) => {
        if (active) {
          setFolderLoad("error");
          const invalid = validationError(e);
          if (invalid) setFieldError(invalid);
          else setFolderError(message(e));
        }
      });
    return () => {
      active = false;
    };
  }, [step, sessionID, path, savedID, folderRequest]);
  const close = () => {
    if (!busy)
      void action(async () => {
        if (sessionID) await api.end(sessionID);
        onClose();
      });
  };
  const dialog = useDialog(close);
  useEffect(() => {
    if (!fieldError || busy || loading) return;
    const input = Array.from(dialog.current?.querySelectorAll<HTMLElement>("[data-field]") || [])
      .find((element) => element.dataset.field === fieldError.field);
    if (!input) {
      setError(localizedMessage(fieldError) || fieldError.message);
      return;
    }
    let parent = input.parentElement;
    while (parent && parent !== dialog.current) {
      if (parent instanceof HTMLDetailsElement) parent.open = true;
      parent = parent.parentElement;
    }
    input.focus();
  }, [fieldError, busy, loading]);
  useEffect(() => {
    const heading = dialog.current?.querySelector<HTMLElement>("h3");
    if (heading) {
      heading.tabIndex = -1;
      heading.focus();
    } else
      dialog.current
        ?.querySelector<HTMLElement>("button,input,select")
        ?.focus();
  }, [step]);
  const unavailable =
    !initial.id &&
    p.available === false &&
    !(p.id === "gcs" && values.service_account_file);
  const noLetters = !initial.id && !savedID && !data.letters.length;
  const savedConnection = data.connections.find((c) => c.id === savedID);
  const letterLocked = initial.recovery || !!savedConnection?.recovery || !!savedConnection?.mounted || !!savedConnection?.busy;
  const letters = Array.from(new Set([
    ...((initial.id || savedID) ? [connection.letter] : []), ...data.letters,
  ])).sort();
  const question = session?.question;
  const destinationLocked = initial.recovery || !!savedID || busy;
  const visibleFolders = folders.filter((item) => item.name.toLowerCase().includes(filter.toLowerCase()));
  const navigate = (next: string) => {
    setPath(next);
    setDirectPath(next);
    setFilter("");
    setFieldError(undefined);
    setFolderError("");
    setFolderLoad("loading");
    setFolderRequest((request) => request + 1);
  };
  const validationProps = (key: string, help?: string) => ({
    "data-field": key,
    "aria-invalid": fieldError?.field === key,
    "aria-describedby": [help, fieldError?.field === key ? `${fieldID}-${key}-error` : undefined].filter(Boolean).join(" ") || undefined,
  });
  const fieldFeedback = (key: string) => fieldError?.field === key && (
    <p id={`${fieldID}-${key}-error`} className="inline-error" role="alert">{t(localizedMessage(fieldError) || fieldError.message)}</p>
  );
  const setValue = (key: string, value: string) => {
    setValues((v) => ({ ...v, [key]: value }));
    if (fieldError?.field === key) setFieldError(undefined);
    if (["host", "port"].includes(key)) {
      setIdentity(undefined);
      setTrusted(false);
    }
  };
  const validateName = () => {
    const invalid = driveNameError(connection.name, auth && !initial.id ? p.name : "");
    if (invalid) setFieldError({ field: "name", message: invalid });
    return !invalid;
  };
  const advance = () => {
    if (validateName()) dispatch({ type: "access" });
  };
  const selectMethod = (key: string) => {
    const method = serviceMethods.find((m) => m.key === key);
    if (!method) return;
    change({ provider: method.provider.id, authMethod: method.authMethod, name: "", sessionOnly: false, alternateURL: "", connectTimeout: 0, idleTimeout: 0, useProxy: false });
    setValues({}); setIdentity(undefined); setTrusted(false); setTransport("explicit"); setInsecure(false);
  };
  const selectCategory = (next: string) => {
    setCategory(next);
    const matching = serviceMethods.filter((m) => m.advanced === (next === "advanced"));
    if (!matching.some((m) => m.key === methodKey(connection, data.providers)) && matching[0]) selectMethod(matching[0].key);
  };
  const update = () => action(async () => {
    if (!validateName()) return;
    await api.update({ ...connection, name: trimDriveName(connection.name) });
    onClose();
  });
  const begin = () =>
    action(async () => {
      if (!validateName()) return;
      if (noLetters)
        throw new Error("No drive letters are available. Free a letter between D and Z before creating a connection.");
      if (!fieldsLoaded)
        throw new Error("Could not load access details. Cancel and try again.");
      const populated = { ...values };
      for (const f of p.fields)
        if (populated[f.key] === undefined && f.default)
          populated[f.key] = f.default;
      const id = await api.begin({
        connection: { ...connection, name: trimDriveName(connection.name) || p.name },
        values: populated,
        transport,
        allowInsecure: insecure,
        hostKey: trusted ? identity?.key || "" : "",
        guided: true,
        ...(forgetCredentials ? { forgetCredentials: true } : {}),
      });
      setValues({});
      dispatch({ type: "start", sessionID: id });
    });
  const answerQuestion = () => action(async () => {
    await api.answer(sessionID, answer);
    setAnswer("");
    dispatch({ type: "answering", sessionID });
  });
  const connect = () => action(async () => {
    try {
      let id = savedID;
      if (!id || savedLetter !== connection.letter) {
        setOperation("validating");
        id = await api.confirm(sessionID, path, connection.letter);
        dispatch({ type: "saved", id, letter: connection.letter });
      }
      setOperation("mounting");
      await api.mount(id);
      dispatch({ type: "connected" });
    } finally {
      await onRefresh();
    }
  });
  return {
    advance, update, initial, auth, data, t, connection,
    values, step, transport, setTransport, insecure, setInsecure, identity,
    setIdentity, trusted, setTrusted, sessionID, session, answer, setAnswer,
    busy, loading, error, fieldError, setFieldError, fieldID, folderError,
    filter, setFilter, folders, path, directPath, setDirectPath, savedID,
    operation, p, change, action, close, dialog, unavailable,
    noLetters, letterLocked, letters, question, destinationLocked, visibleFolders, navigate,
    validationProps, fieldFeedback, setValue, begin, dispatch, setValues, fieldsLoaded,
    setFolderLoad, setFolderRequest, answerQuestion, connect,
    category, selectCategory, serviceMethods, selectMethod, browsers, forgetCredentials, setForgetCredentials,
  };
}

export type Wizard = ReturnType<typeof useConnectionWizard>;
