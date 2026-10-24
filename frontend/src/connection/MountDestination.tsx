import { api } from "../api";
import type { Wizard } from "./useConnectionWizard";

export function MountDestination({ wizard }: { wizard: Wizard }) {
  const { t, connection: c, change, busy, letterLocked, letters, validationProps, fieldFeedback, action } = wizard;
  const folder = c.mountFolder !== undefined && c.letter === "";
  return <fieldset disabled={busy || letterLocked}>
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
      <label>{t("Local mount folder")}<input disabled={busy || letterLocked} {...validationProps("mountFolder")} required value={c.mountFolder || ""} onChange={(e) => change({ mountFolder: e.target.value })} /></label>
      <button type="button" onClick={() => void action(async () => {
        const parent = await api.selectDirectory();
        if (parent) change({ mountFolder: parent.replace(/[\\/]+$/, "") + "\\SDrive" });
      })}>{t("Choose parent directory")}</button>
      <small>{t("Choose an existing parent and a new subfolder name. The final path must not exist; SDrive never removes existing contents.")}</small>
      {fieldFeedback("mountFolder")}
    </> : <>
      <label>{t("Windows drive letter")}<select disabled={busy || letterLocked || !letters.length} {...validationProps("letter")} aria-label={t("Windows drive letter")} value={letters.includes(c.letter) ? c.letter : ""} onChange={(e) => change({ letter: e.target.value })}>
        {!letters.includes(c.letter) && <option value="" disabled>{t("Select…")}</option>}
        {letters.filter(Boolean).map((letter) => <option key={letter} value={letter}>{letter}:</option>)}
      </select></label>
      {fieldFeedback("letter")}
      <small>{t(letterLocked ? "Keep the saved letter while recovery or an operation is pending. Resolve pending files before changing it." : "Choose a free letter. If connecting fails, you can correct it here and retry the saved connection.")}</small>
    </>}
    <label>{t("Drive presentation")}<select {...validationProps("mountMode")} value={c.mountMode || "network"} disabled={folder || busy || letterLocked} onChange={(e) => change({ mountMode: e.target.value })}>
      <option value="network">{t("Network drive")}</option>
      <option value="local">{t("Local disk")}</option>
    </select></label>
    {fieldFeedback("mountMode")}
    <small>{t("Folder mounts require local-disk mode. SDrive runs in your Windows session; elevated applications may see different drives. Local-disk access is restricted to your user.")}</small>
  </fieldset>;
}
