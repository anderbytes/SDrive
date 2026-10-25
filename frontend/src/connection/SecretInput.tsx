import { useId, useState } from "react";
import { useLanguage } from "../i18n";

export function SecretInput({ label, value, required, onChange, validation, describedBy }: {
  label: string;
  value: string;
  required: boolean;
  onChange: (value: string) => void;
  validation?: { "data-field": string; "aria-invalid": boolean; "aria-describedby": string | undefined };
  describedBy?: string;
}) {
  const { t } = useLanguage();
  const id = useId();
  const [revealed, setRevealed] = useState(false);
  return (
    <div className="secret-input">
      <label htmlFor={id}>{label}</label>
      <div className="secret-control">
        <input data-sensitive id={id} type={revealed ? "text" : "password"} autoComplete="off"
          {...validation}
          aria-describedby={[describedBy, validation?.["aria-describedby"]].filter(Boolean).join(" ") || undefined}
          required={required} value={value} onChange={(e) => onChange(e.target.value)} />
        <button type="button" aria-controls={id} aria-pressed={revealed}
          aria-label={t(revealed ? "Hide {field}" : "Show {field}", { field: label })}
          onClick={() => setRevealed((show) => !show)}>
          {t(revealed ? "Hide" : "Show")}
        </button>
      </div>
    </div>
  );
}
