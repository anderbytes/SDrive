import { api } from "../api";
import { SecretInput } from "./SecretInput";
import { FieldHelp } from "../FieldHelp";
import type { Wizard } from "./useConnectionWizard";

export function AccessFields({ advanced, wizard }: { advanced: boolean; wizard: Wizard }) {
  const {
    t, values, busy, fieldID, p, action, validationProps,
    fieldFeedback, setValue,
  } = wizard;
  return p.fields
      .filter(
        (f) =>
          (!(p.id === "gcs") || (wizard.connection.authMethod === "serviceAccount" ? !["client_id", "client_secret"].includes(f.key) : f.key !== "service_account_file")) &&
          (f.key !== "key_file_pass" || !!values.key_file) && !!f.advanced === advanced,
      )
      .map((f) => (
        <div className="credential-field" key={`${p.id}-${f.key}`}>
          {f.secret ? (
            <SecretInput label={t(f.label)}
              validation={validationProps(f.key, f.hint ? `${fieldID}-${f.key}-help` : undefined)}
              value={values[f.key] ?? f.default ?? ""} required={f.required || (f.key === "client_secret" && !wizard.initial.id)}
              onChange={(value) => setValue(f.key, value)} />
          ) : <label>
          <span className="field-label">{t(f.label)}</span>
          {f.choices ? (
            <select
              disabled={["shared_with_me", "export_formats"].includes(f.key) && wizard.initial.recovery}
              {...validationProps(f.key, f.hint ? `${fieldID}-${f.key}-help` : undefined)}
              value={values[f.key] ?? f.default}
              onChange={(e) => setValue(f.key, e.target.value)}
            >
              {f.choices.map((value) => (
                <option key={value} value={value}>
                  {value === "api.pcloud.com"
                    ? t("United States")
                    : value === "eapi.pcloud.com"
                      ? t("Europe")
                      : f.key === "export_formats" ? t(value === "docx,xlsx,pptx,svg" ? "Office formats (default)" : value === "odt,ods,odp,svg" ? "OpenDocument formats" : "PDF preferred (with fallback)")
                      : f.key === "shared_with_me" ? t(value === "true" ? "Shared with me" : "My Drive / selected library") : value}
                </option>
              ))}
            </select>
          ) : (
            <input
              {...validationProps(f.key, f.hint ? `${fieldID}-${f.key}-help` : undefined)}
              type="text"
              autoComplete="off"
              required={f.required || f.key === "client_id" || (f.key === "service_account_file" && wizard.connection.authMethod === "serviceAccount")}
              value={values[f.key] ?? f.default ?? ""}
              onChange={(e) => setValue(f.key, e.target.value)}
            />
          )}
          </label>}
          {fieldFeedback(f.key)}
          {f.hint && <FieldHelp id={`${fieldID}-${f.key}-help`} label={t(f.label)}>{t(f.hint)}</FieldHelp>}
          {f.file && (
            <button
              type="button"
              disabled={busy}
              onClick={() =>
                void action(async () => {
                  const file = await api.selectFile(f.key);
                  if (file) setValue(f.key, file);
                })
              }
            >
              {t("Select file…")}
            </button>
          )}
        </div>
      ));
}
