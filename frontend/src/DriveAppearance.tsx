import { useEffect, useRef, useState } from "react";
import { api, type Connection, type DriveAppearance as Appearance } from "./api";
import { DialogLayout } from "./DialogLayout";
import { useDialog } from "./useDialog";
import { useLanguage } from "./i18n";
import { errorMessage } from "./messages";
import { LoadingStatus } from "./Spinner";

import { driveColors, driveColorStyle } from "./driveColors";

export function DriveIcon({ connection: c }: { connection: Pick<Connection, "iconImage" | "mountFolder" | "letter"> }) {
  const { t } = useLanguage();
  const [failed, setFailed] = useState(false);
  useEffect(() => setFailed(false), [c.iconImage]);
  return <div className="letter" role="img" aria-label={c.mountFolder ? t("Local folder") : `${t("Drive letter")} ${c.letter}:`}>
    {c.iconImage && !failed ? <img src={c.iconImage} alt="" onError={() => setFailed(true)} /> : c.mountFolder
      ? <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7"><path d="M3 7V5a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Z" /><path d="M3 8h18" /></svg>
      : <>{c.letter}<span>:</span></>}
  </div>;
}

export async function normalizeDriveImage(file: File): Promise<string> {
  if (!["image/png", "image/jpeg", "image/webp"].includes(file.type) || file.size > 5 * 1024 ** 2) {
    throw new Error("Choose a PNG, JPEG or WebP image up to 5 MiB.");
  }
  try {
    const source = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.onerror = () => reject(new Error("The image could not be read."));
      reader.readAsDataURL(file);
    });
    const image = new Image();
    image.src = source;
    await image.decode();
    if (!image.naturalWidth || !image.naturalHeight) throw new Error("The image could not be read.");
    const scale = Math.min(1, 128 / Math.max(image.naturalWidth, image.naturalHeight));
    const canvas = document.createElement("canvas");
    canvas.width = Math.max(1, Math.round(image.naturalWidth * scale));
    canvas.height = Math.max(1, Math.round(image.naturalHeight * scale));
    const context = canvas.getContext("2d");
    if (!context) throw new Error("The image could not be read.");
    context.drawImage(image, 0, 0, canvas.width, canvas.height);
    const result = canvas.toDataURL("image/png");
    if (!result.startsWith("data:image/png;base64,") || result.length > 128 * 1024) throw new Error("The image could not be read.");
    return result;
  } catch {
    throw new Error("The image could not be read.");
  }
}

export function DriveAppearanceDialog({ connection, unavailable, onClose, onSave }: {
  connection: Connection;
  unavailable: boolean;
  onClose: () => void;
  onSave: () => void;
}) {
  const { t } = useLanguage();
  const [appearance, setAppearance] = useState<Appearance>({ cardStyle: connection.cardStyle || "", iconImage: connection.iconImage || "" });
  const [choosingColor, setChoosingColor] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const active = useRef(true);
  useEffect(() => { active.current = true; return () => { active.current = false; }; }, []);
  const close = () => { if (!busy && !choosingColor) onClose(); };
  const dialog = useDialog(close);
  const save = async () => {
    setBusy(true); setError("");
    try {
      await api.updateDriveAppearance(connection.id, appearance);
      onSave();
      onClose();
    } catch (e) { if (active.current) setError(t(errorMessage(e))); }
    finally { if (active.current) setBusy(false); }
  };
  return <DialogLayout dialogRef={dialog} titleID="appearance-title" title={t("Customize appearance")} onClose={close} closeDisabled={busy}
    footer={<><button disabled={busy || choosingColor} onClick={close}>{t("Cancel")}</button><button className="primary" disabled={busy || unavailable || choosingColor} onClick={() => void save()}>{t("Save changes")}</button></>}>
    {choosingColor && <DriveColorDialog selected={appearance.cardStyle || ""} unavailable={unavailable || busy}
      onClose={() => setChoosingColor(false)} onChoose={cardStyle => { setAppearance({ ...appearance, cardStyle }); setChoosingColor(false); }} />}
    {busy && <LoadingStatus>{t("Operation in progress…")}</LoadingStatus>}
    <div className="appearance-color-field">
      <span>{t("Card background")}</span>
      <span className="appearance-color-selection"><span className="appearance-color-chip" aria-hidden="true" style={driveColorStyle(appearance.cardStyle)} />{t(driveColors.find(([id]) => id === appearance.cardStyle)?.[1] || "Default")}</span>
      <button disabled={busy || unavailable} onClick={() => setChoosingColor(true)}>{t("Choose color…")}</button>
    </div>
    <label>{t("Drive image")}<input type="file" accept="image/png,image/jpeg,image/webp" disabled={busy || unavailable}
      onChange={async (event) => {
        const file = event.currentTarget.files?.[0];
        event.currentTarget.value = "";
        if (!file) return;
        setBusy(true); setError("");
        try { const iconImage = await normalizeDriveImage(file); if (active.current) setAppearance((value) => ({ ...value, iconImage })); }
        catch (e) { if (active.current) setError(t(errorMessage(e))); }
        finally { if (active.current) setBusy(false); }
      }} /></label>
    <p className="hint">{t("PNG, JPEG or WebP, up to 5 MiB. A small copy is saved in SDrive.")}</p>
    {error && <p role="alert" className="alert error">{t(error)}</p>}
    <section className="appearance-preview-section" aria-label={t("Preview")}>
      <div className="appearance-preview-heading"><strong>{t("Preview")}</strong>
        <button className="text-button" disabled={busy || unavailable || !appearance.iconImage} onClick={() => setAppearance({ ...appearance, iconImage: "" })}>{t("Remove image")}</button>
      </div>
      <div className="appearance-preview" data-card-style={appearance.cardStyle} style={driveColorStyle(appearance.cardStyle)}>
        <DriveIcon connection={{ ...connection, iconImage: appearance.iconImage }} /><span>{t("Preview")}</span>
      </div>
    </section>
  </DialogLayout>;
}


function DriveColorDialog({ selected, unavailable, onClose, onChoose }: {
  selected: string;
  unavailable: boolean;
  onClose: () => void;
  onChoose: (style: string) => void;
}) {
  const { t } = useLanguage();
  const [color, setColor] = useState(selected);
  const dialog = useDialog(onClose);
  return <DialogLayout centered dialogRef={dialog} titleID="drive-color-title" title={t("Choose card color")} onClose={onClose}
    footer={<><button onClick={onClose}>{t("Cancel")}</button><button className="primary" disabled={unavailable} onClick={() => onChoose(color)}>{t("Use color")}</button></>}>
    <fieldset className="appearance-styles" disabled={unavailable}><legend>{t("Card background")}</legend>
      {driveColors.map(([id, label]) => <label key={id} className="appearance-swatch" style={driveColorStyle(id)}>
        <input type="radio" name="card-style" value={id} checked={color === id} onChange={() => setColor(id)} />{t(label)}
      </label>)}
    </fieldset>
  </DialogLayout>;
}
