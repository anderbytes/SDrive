import { MountDestination } from "./MountDestination";
import type { Wizard } from "./useConnectionWizard";
import { LoadingStatus } from "../Spinner";

export function FolderStep({ wizard }: { wizard: Wizard }) {
  const {
    initial, data, t, busy, loading, fieldError,
    setFieldError, fieldID, folderError, filter, setFilter, folders, path,
    directPath, setDirectPath, savedID,
    destinationLocked, visibleFolders, navigate, validationProps, fieldFeedback,
    setFolderLoad, setFolderRequest,
  } = wizard;
  return (
    <div className="reveal-content">
      <h3>{t("Choose what to connect")}</h3>
      <p>{t("Select a folder or keep the entire storage.")}</p>
      <p aria-live="polite">
        <strong>{t("Current folder:")}</strong>{" "}
        {path || t("Entire storage")}
      </p>
      <nav className="folder-breadcrumbs" aria-label={t("Folder navigation")}>
        <button disabled={destinationLocked} aria-current={!path ? "location" : undefined}
          onClick={() => navigate("")}>{t("Entire storage")}</button>
        {path.split("/").map((part, index, parts) => part && (
          <span key={index}>
            <span aria-hidden="true"> / </span>
            <button disabled={destinationLocked} aria-current={index === parts.length - 1 ? "location" : undefined}
              onClick={() => navigate(parts.slice(0, index + 1).join("/"))}>{part}</button>
          </span>
        ))}
      </nav>
      {folderError && (
        <div>
          <p role="alert">
            {t(folderError)}{" "}
            {t("If your account only allows access to a specific destination, enter its address below.")}
          </p>
          <button disabled={busy || loading || !!savedID}
            onClick={() => { setFolderLoad("loading"); setFolderRequest((request) => request + 1); }}>
            {t("Retry folder listing")}
          </button>
        </div>
      )}
      {!initial.recovery && <label>
        {t("Filter folders")}
        <input type="search" value={filter} disabled={destinationLocked}
          onChange={(e) => setFilter(e.target.value)} />
      </label>}
      {loading ? (
        <LoadingStatus>{t("Loading folders…")}</LoadingStatus>
      ) : (
        <div className="folder-list" aria-label={t("Available folders")}>
          {path && !initial.recovery && (
            <button
              disabled={destinationLocked}
              onClick={() =>
                navigate(path.replace(/\/$/, "").split("/").slice(0, -1).join("/"))
              }
            >
              {t("↑ Parent folder")}
            </button>
          )}
          {!initial.recovery &&
            visibleFolders.map((item) => (
              <button
                key={item.path}
                disabled={destinationLocked}
                onClick={() => navigate(item.path)}
              >
                ▱ {item.name}
              </button>
            ))}
          {!folderError && fieldError?.field !== "path" && folders.length === 0 && (
            <p>{t("No subfolders here.")}</p>
          )}
          {!folderError && folders.length > 0 && visibleFolders.length === 0 && (
            <p role="status">{t("No folders match your filter.")}</p>
          )}
        </div>
      )}
      {!initial.recovery && (
        <details>
          <summary>{t("Enter destination directly")}</summary>
          <div className="field-action">
          <label>
            {t("Folder, bucket or share")}
            <input
              {...validationProps("path", `${fieldID}-path-help`)}
              value={directPath}
              disabled={destinationLocked}
              placeholder={t("Example: documents or my-bucket/folder")}
              onChange={(e) => {
                setDirectPath(e.target.value);
                if (fieldError?.field === "path") setFieldError(undefined);
              }}
            />
          </label>
          <button
            disabled={destinationLocked}
            onClick={() => navigate(directPath)}
          >
            {t("Open destination")}
          </button>
          </div>
          <small id={`${fieldID}-path-help`}>{t("Use the path provided by your administrator, separated by /.")}</small>
          {fieldFeedback("path")}
        </details>
      )}
      {directPath !== path && <p role="status">{t("Destination not applied. Choose Open destination before connecting.")}</p>}
      <MountDestination wizard={wizard} collapsed />
      {!data.winFsp && (
        <p role="alert">
          {t("Run the SDrive installer again to prepare drive mounting.")}
        </p>
      )}
    </div>
  );
}

export function FolderActions({ wizard }: { wizard: Wizard }) {
  const { t, busy, loading, directPath, path, fieldError, data, noLetters, connection, savedID, initial, connect, close, operation } = wizard;
  return <>
    <button disabled={busy} onClick={close}>{savedID ? t("Close") : t("Cancel")}</button>
    <button className="primary" disabled={busy || loading || directPath !== path || fieldError?.field === "path" || !data.winFsp || noLetters || (connection.letter !== "" && !savedID && !initial.id && !data.letters.includes(connection.letter))}
      onClick={() => void connect()}>{busy ? t(operation === "mounting" ? "Mounting drive…" : "Validating destination…") : savedID ? t("Retry connection") : t("Connect drive")}</button>
  </>;
}
