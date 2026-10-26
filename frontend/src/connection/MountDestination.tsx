import { FieldHelp } from "../FieldHelp";
import { useEffect, useRef } from "react";
import { api } from "../api";
import type { Wizard } from "./useConnectionWizard";

export function MountDestination({ wizard, collapsed = false }: { wizard: Wizard; collapsed?: boolean }) {
  const { t, connection: c, change, busy, letterLocked, letters, validationProps, fieldFeedback, action } = wizard;
  const folder = c.mountFolder !== undefined && c.letter === "";
  const disclosure = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    if (disclosure.current && (wizard.noLetters || wizard.fieldError && ["letter", "mountFolder", "mountMode"].includes(wizard.fieldError.field))) disclosure.current.open = true;
  }, [wizard.noLetters, wizard.fieldError]);
  const fields = <fieldset disabled={busy || letterLocked}>
    <legend>{t("Windows mount destination")}</legend>
    <label>{t("Mount into")}
      <select disabled={busy || letterLocked} value={folder ? "folder" : "letter"} onChange={(e) => change(e.target.value === "folder"
        ? { letter: "", mountFolder: "", mountMode: "local" }
        : { letter: letters[0] || "S", mountFolder: undefined })}>
        <option value="letter">{t("Drive letter")}</option>
        <option value="folder">{t("New local subfolder")}</option>
      </select>
    </label>
    {folder ? <>
      <div className="field-with-help"><label><span className="field-label">{t("Local mount folder")}</span><input disabled={busy || letterLocked} {...validationProps("mountFolder")} required value={c.mountFolder || ""} onChange={(e) => change({ mountFolder: e.target.value })} /></label>
      <FieldHelp label={t("Local mount folder")}>{t("Choose an existing parent and a new subfolder name. The final path must not exist; SDrive never removes existing contents.")}</FieldHelp></div>
      <button type="button" onClick={() => void action(async () => {
        const parent = await api.selectDirectory();
        if (parent) change({ mountFolder: parent.replace(/[\\/]+$/, "") + "\\SDrive" });
      })}>{t("Choose parent directory")}</button>
      {fieldFeedback("mountFolder")}
    </> : <>
      <div className="field-with-help"><label><span className="field-label">{t("Windows drive letter")}</span><select disabled={busy || letterLocked || !letters.length} {...validationProps("letter")} aria-label={t("Windows drive letter")} value={letters.includes(c.letter) ? c.letter : ""} onChange={(e) => change({ letter: e.target.value })}>
        {!letters.includes(c.letter) && <option value="" disabled>{t("Select…")}</option>}
        {letters.filter(Boolean).map((letter) => <option key={letter} value={letter}>{letter}:</option>)}
      </select></label>
      {fieldFeedback("letter")}
      <FieldHelp label={t("Windows drive letter")}>{t("Choose a free letter. If connecting fails, you can correct it here and retry the saved connection.")}</FieldHelp></div>
      {letterLocked && <p role="status">{t("Keep the saved letter while recovery or an operation is pending. Resolve pending files before changing it.")}</p>}
    </>}
    <div className="field-with-help">{folder ? <p>{t("Drive presentation")} · {t("Local disk")}</p> : <label><span className="field-label">{t("Drive presentation")}</span><select {...validationProps("mountMode")} value={c.mountMode || "network"} disabled={busy || letterLocked} onChange={(e) => change({ mountMode: e.target.value })}>
      <option value="network">{t("Network drive")}</option>
      <option value="local">{t("Local disk")}</option>
    </select></label>}
    {fieldFeedback("mountMode")}
    <FieldHelp label={t("Drive presentation")}>{t("Folder mounts require local-disk mode. SDrive runs in your Windows session; elevated applications may see different drives. Local-disk access is restricted to your user.")}</FieldHelp>
    </div>
  </fieldset>;
  return collapsed ? <details ref={disclosure} className="mount-summary">
    <summary>{t("Windows mount destination")} <span className="mount-target">{folder ? c.mountFolder : `${c.letter}:`}</span></summary>
    {fields}
  </details> : fields;
}
