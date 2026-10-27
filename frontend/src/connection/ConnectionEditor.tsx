import { flushSync } from "react-dom";
import { MountDestination } from "./MountDestination";
import { TransportCapability } from "../TransportSecurity";
import { advancedDisclaimer, methodKey, oauthInstructions, supportsNetworkOptions } from "./methods";
import { ConnectionOptions, DirectoryCacheOptions, NetworkOptions } from "./ConnectionOptions";
import { api } from "../api";
import { useConnectionWizard, type EditorProps } from "./useConnectionWizard";
import { AccessFields } from "./AccessFields";
import { AuthenticationActions, AuthenticationStep } from "./AuthenticationStep";
import { FolderActions, FolderStep } from "./FolderStep";
import { DialogLayout } from "../DialogLayout";
import { DialogNavigation } from "../DialogNavigation";
import { FieldHelp } from "../FieldHelp";
import { LoadingStatus } from "../Spinner";

export function ConnectionEditor(props: EditorProps) {
  const wizard = useConnectionWizard(props);
  const {
    advance, update, initial, auth, data, t, connection,
    values, step, transport, setTransport, insecure, setInsecure, identity,
    setIdentity, trusted, setTrusted, session, busy, loading, error,
    fieldID, savedID, p, change, action, close, dialog,
    unavailable, noLetters, validationProps, fieldFeedback, begin, dispatch,
    fieldsLoaded, editTab, setEditTab, operation,
  } = wizard;
  const formID = `${fieldID}-form`;
  const footer = step === "folder" ? <FolderActions wizard={wizard} /> : step === "auth" ? <AuthenticationActions wizard={wizard} /> : step === "complete" ? <>
    <button onClick={close}>{t("Done")}</button><button className="primary" disabled={busy} title={t("Open in Explorer")} onClick={() => void action(() => api.open(savedID))}>{t("Open")}</button>
  </> : <>
    <button type="button" disabled={busy} onClick={() => step === "access" && !initial.id ? dispatch({ type: "service" }) : close()}>{step === "access" && !initial.id ? t("Back") : t("Cancel")}</button>
    <button form={formID} className="primary" disabled={step === "service"
      ? !wizard.serviceMethods.some((m) => m.key === methodKey(connection, data.providers) && m.advanced === (wizard.category === "advanced"))
      : step === "access" ? busy || loading || !fieldsLoaded || noLetters || unavailable || (p.id === "sftp" && !trusted) : busy}>
      {step === "edit" ? t("Save changes") : busy || loading ? t("Preparing…") : step === "access" && p.oauth && !values.service_account_file ? t("Sign in with {provider}", { provider: p.name }) : t("Continue")}
    </button>
  </>;
  const setupSection = step === "service" ? "Service" : ["access", "auth"].includes(step) ? "Access"
    : step === "complete" || savedID || busy && step === "folder" ? "Connect" : "Folder";
  const navigation = step === "edit" ? <DialogNavigation label={t("Drive settings")} selected={editTab} onSelect={setEditTab}
    items={["general", "files", "network"].filter((key) => key !== "network" || supportsNetworkOptions(connection.provider)).map((key) => ({ key, label: t(key === "general" ? "General" : key === "files" ? "Files" : "Network"),
      id: `${fieldID}-edit-${key}-tab`, panel: `${fieldID}-edit-${key}-panel`, disabled: busy }))} />
    : <DialogNavigation steps label={t("Connection setup")} selected={setupSection}
      onSelect={(key) => { if (key === "Service" && step === "access" && !initial.id && !busy) dispatch({ type: "service" }); }}
      items={["Service", "Access", "Folder", "Connect"].map((key) => ({ key, label: t(key),
        disabled: key !== setupSection && !(key === "Service" && step === "access" && !initial.id && !busy) }))} />;
  return (
    <DialogLayout dialogRef={dialog} titleID="editor-title" title={initial.id ? auth ? t("Reconfigure account") : t("Edit drive") : t("New connection")}
      navigation={navigation} eyebrow={auth ? undefined : t("CONFIGURE DRIVE")} onClose={close} closeDisabled={busy} closeLabel={t("Close setup")} footer={footer}>
        {step !== "auth" && (busy || loading && step !== "folder") && <LoadingStatus>{t(operation === "mounting" ? "Mounting drive…" : operation === "validating" ? "Validating destination…" : "Operation in progress…")}</LoadingStatus>}
        {(error || session?.error) && (
          <div role="alert" className="alert error">
            {t(error || session?.error || "")}
          </div>
        )}
        {noLetters && <p role="alert">{t("No drive letters are available. Choose a new local subfolder or free a drive letter.")}</p>}
        {savedID && step === "folder" && (
          <p role="status" className="alert warning">{t("Configuration saved. Retry uses this connection without creating another. You can also close setup and connect from My drives.")}</p>
        )}
        {initial.recovery && (
          <div className="alert warning">
            {t(
              "Only renew access to the same account. The original destination will be kept to recover your pending files.",
            )}
          </div>
        )}
        {step === "service" && (
          <form
            id={formID}
            onSubmit={(e) => {
              e.preventDefault();
              advance();
            }}
          >
            <div role="tablist" aria-label={t("Connection setup category")}>
              {["basic", "advanced"].map((category) => <button key={category} id={`${fieldID}-${category}-tab`} type="button" role="tab" aria-selected={wizard.category === category} aria-controls={`${fieldID}-services`} tabIndex={wizard.category === category ? 0 : -1}
                onClick={() => wizard.selectCategory(category)}
                onKeyDown={(e) => { if (["ArrowLeft", "ArrowRight", "Home", "End"].includes(e.key)) { e.preventDefault(); const next = e.key === "Home" ? "basic" : e.key === "End" ? "advanced" : category === "basic" ? "advanced" : "basic"; wizard.selectCategory(next); document.getElementById(`${fieldID}-${next}-tab`)?.focus(); } }}>
                {t(category === "basic" ? "Basic" : "Advanced")}
              </button>)}
            </div>
            <div id={`${fieldID}-services`} role="tabpanel" aria-labelledby={`${fieldID}-${wizard.category}-tab`}>
              {wizard.category === "advanced" && <p className="alert warning">{t(advancedDisclaimer)}</p>}
              <label>{t("Connection type")}<select value={methodKey(connection, data.providers)} onChange={(e) => wizard.selectMethod(e.target.value)}>
                {wizard.serviceMethods.filter((m) => m.advanced === (wizard.category === "advanced")).map((m) => <option key={m.key} value={m.key}>{t(m.label)}{m.provider.experimental ? ` (${t("experimental")})` : ""}</option>)}
              </select></label>
              {!wizard.serviceMethods.some((m) => m.advanced === (wizard.category === "advanced")) && <p>{t("No connection methods in this category.")}</p>}
            </div>
            <label>
              {t("Drive name")}
              <input
                {...validationProps("name")}
                value={connection.name}
                placeholder={t(p.name)}
                onChange={(e) => change({ name: e.target.value })}
              />
            </label>
            {fieldFeedback("name")}
            {unavailable && <p role="status">{t(p.notice || "")}</p>}
            <MountDestination wizard={wizard} />
          </form>
        )}
        {step === "access" && (
          <form
            id={formID}
            onSubmit={(e) => {
              e.preventDefault();
              void begin();
            }}
          >
            <div className="access-intro"><h3>
              {p.id === "gcs" && connection.authMethod === "serviceAccount"
                ? t("Access to {provider}", { provider: p.name })
                : p.oauth
                ? t("Sign in to {provider}", { provider: p.name })
                : t("Access to {provider}", { provider: p.name })}
            </h3>
            <FieldHelp label={t("Access details")}>
              {p.id === "gcs" && connection.authMethod === "serviceAccount"
                ? t("Select a service account JSON file authorized for your bucket. Browser sign-in is not required.")
                : p.oauth
                ? t(
                    "Sign in through your browser and authorize SDrive. Then return to this window.",
                  )
                : t(
                    "Enter your account credentials or the details provided by your storage administrator.",
                  )}
            </FieldHelp></div>
            {unavailable && (
              <div className="alert warning" role="status">
                {t(p.notice || "")}
              </div>
            )}
            {p.oauth && connection.authMethod !== "serviceAccount" && <>
              <p>{t(oauthInstructions[p.id] || "")}</p>
              <button type="button" onClick={() => void action(() => api.prerequisite(`oauth-${p.id}`))}>{t("Open service setup instructions")}</button>
            </>}
            {p.experimental && (
              <div className="alert warning">
                {t(
                  "This service is still experimental. Compatibility may vary by account.",
                )}
              </div>
            )}
            <fieldset disabled={loading || busy}>
              <legend>{t("Access details")}</legend>
              <AccessFields advanced={false} wizard={wizard} />
              <TransportCapability provider={p} />
              {(p.id === "ftp" && transport === "plain" || /^http:\/\//i.test(values.url || values.endpoint || "")) && <p role="status" className="alert warning">{t("Unencrypted connections expose credentials and file content to the network.")}</p>}
              {p.id === "ftp" && transport === "plain" && <p role="status">{t("This connection uses unencrypted FTP. Confirm permission in Advanced options before continuing.")}</p>}
              {p.id === "sftp" && (
                <div className="host-key">
                  <p id={`${fieldID}-ssh-help`}>
                    {t(
                      "Verify the identity with your administrator before trusting this server.",
                    )}
                  </p>
                  <button
                    type="button"
                    onClick={() =>
                      void action(async () => {
                        setIdentity(
                          await api.probe(values.host || "", values.port || ""),
                        );
                        setTrusted(false);
                      })
                    }
                  >
                    {t("Verify SSH identity")}
                  </button>
                  {identity && (
                    <>
                      <code>{identity.fingerprint}</code>
                      <label className="check">
                        <input
                          type="checkbox"
                          aria-describedby={`${fieldID}-ssh-help`}
                          checked={trusted}
                          onChange={(e) => setTrusted(e.target.checked)}
                        />
                        {t("I verified the fingerprint and trust this server")}
                      </label>
                    </>
                  )}
                </div>
              )}
            </fieldset>
              <details className="advanced-options">
                <summary>{t("Advanced options")}</summary>
                <fieldset className="advanced-content" disabled={loading || busy}>
                {p.fields.some((field) => field.advanced && (field.key !== "key_file_pass" || values.key_file)) && <fieldset className="option-group">
                  <legend>{t("Additional access settings")}</legend>
                  <AccessFields advanced wizard={wizard} />
                </fieldset>}
                <ConnectionOptions wizard={wizard} />
                {["webdav", "nextcloud", "owncloud", "s3", "ftp"].includes(p.id) && <fieldset className="option-group"><legend>{t("Connection security")}</legend>
                {p.id === "ftp" && <p>{t("FTP uses passive transfers only. Active mode, Mode Z compression and legacy filename charsets are not supported.")}</p>}
                {p.id === "ftp" && (
                  <>
                    <label>
                      {t("Access type")}
                      <select
                        aria-describedby={`${fieldID}-transport-help`}
                        value={transport}
                        onChange={(e) => setTransport(e.target.value)}
                      >
                        <option value="explicit">
                          {t("Secure FTPS (default)")}
                        </option>
                        <option value="implicit">{t("Implicit FTPS")}</option>
                        <option value="plain">{t("Unencrypted FTP")}</option>
                      </select>
                    </label>
                    <FieldHelp id={`${fieldID}-transport-help`} label={t("Access type")}>{t("FTPS encrypts the connection. Unencrypted FTP exposes credentials and file content to the network.")}</FieldHelp>
                  </>
                )}
                {["webdav", "nextcloud", "owncloud", "s3", "ftp"].includes(
                  p.id,
                ) && (
                  <>
                    <div className="option-with-help"><label className="check">
                      <input
                        type="checkbox"
                        aria-describedby={`${fieldID}-insecure-help`}
                        checked={insecure}
                        onChange={(e) => setInsecure(e.target.checked)}
                      />
                      {t("Allow unencrypted connection (HTTP / FTP)")}
                    </label>
                    <FieldHelp id={`${fieldID}-insecure-help`} label={t("Allow unencrypted connection (HTTP / FTP)")}>{t("Unencrypted connections expose credentials and file content to the network.")}</FieldHelp></div>
                  </>
                )}
                </fieldset>}
                <fieldset className="option-group"><legend>{t("Drive behavior")}</legend>
                <div className="option-with-help"><label className="check">
                  <input
                    type="checkbox"
                    aria-describedby={`${fieldID}-readonly-help`}
                    checked={connection.readOnly}
                    disabled={initial.recovery}
                    onChange={(e) => change({ readOnly: e.target.checked })}
                  />
                  {t("Mount read only")}
                </label>
                <FieldHelp id={`${fieldID}-readonly-help`} label={t("Mount read only")}>{t("Prevents changes through this drive. Account permissions remain unchanged.")}</FieldHelp></div>
                {!connection.sessionOnly && <>
                <div className="option-with-help"><label className="check">
                  <input
                    type="checkbox"
                    aria-describedby={`${fieldID}-automount-help`}
                    disabled={connection.sessionOnly}
                    checked={connection.autoMount}
                    onChange={(e) => change({ autoMount: e.target.checked })}
                  />
                  {t("Connect automatically when SDrive starts")}
                </label>
                <FieldHelp id={`${fieldID}-automount-help`} label={t("Connect automatically when SDrive starts")}>{t("SDrive attempts to connect this drive when it starts. Recovery may require your attention.")}</FieldHelp></div>
                </>}
                </fieldset>
                </fieldset>
              </details>
          </form>
        )}
        {step === "auth" && <AuthenticationStep wizard={wizard} />}
        {step === "folder" && <FolderStep wizard={wizard} />}
        {step === "complete" && (
          <div className="auth-flow">
            <div className="complete-icon">✓</div>
            <h3>{t("Drive connected")}</h3>
            <p>
              {t("Your files are available at {target}.", { target: connection.mountFolder || `${connection.letter}:` })}
            </p>
          </div>
        )}
        {step === "edit" && (
          <form
            id={formID}
            onInvalidCapture={(e) => {
              const firstInvalid = e.currentTarget.querySelector("input:invalid, select:invalid, textarea:invalid");
              if (e.target !== firstInvalid) {
                e.preventDefault();
                return;
              }
              const tab = firstInvalid?.closest<HTMLElement>("[data-edit-tab]")?.dataset.editTab;
              if (tab && tab !== editTab) flushSync(() => setEditTab(tab));
            }}
            onSubmit={(e) => {
              e.preventDefault();
              void update();
            }}
          >
            <div id={`${fieldID}-edit-general-panel`} role="tabpanel" aria-labelledby={`${fieldID}-edit-general-tab`} data-edit-tab="general" hidden={editTab !== "general"}>
              <label>
                {t("Drive name")}
                <input
                  {...validationProps("name")}
                  required
                  value={connection.name}
                  onChange={(e) => change({ name: e.target.value })}
                />
              </label>
              {fieldFeedback("name")}
              <MountDestination wizard={wizard} />
              <div className="option-with-help"><label className="check">
                <input
                  type="checkbox"
                  aria-describedby={`${fieldID}-readonly-help`}
                  checked={connection.readOnly}
                  disabled={connection.provider === "drive"}
                  onChange={(e) => change({ readOnly: e.target.checked })}
                />
                {t("Mount read only")}
              </label>
              <FieldHelp id={`${fieldID}-readonly-help`} label={t("Mount read only")}>{t("Prevents changes through this drive. Account permissions remain unchanged.")}</FieldHelp></div>
              {connection.provider === "drive" && <p>{t("Use Reconfigure account to change Google Drive access mode and update OAuth consent.")}</p>}
              {!connection.sessionOnly && <>
              <div className="option-with-help"><label className="check">
                <input
                  type="checkbox"
                  aria-describedby={`${fieldID}-automount-help`}
                  disabled={connection.sessionOnly}
                      checked={connection.autoMount}
                  onChange={(e) => change({ autoMount: e.target.checked })}
                />
                {t("Connect automatically when SDrive starts")}
              </label>
              <FieldHelp id={`${fieldID}-automount-help`} label={t("Connect automatically when SDrive starts")}>{t("SDrive attempts to connect this drive when it starts. Recovery may require your attention.")}</FieldHelp></div>
              </>}
            </div>
            <div id={`${fieldID}-edit-files-panel`} role="tabpanel" aria-labelledby={`${fieldID}-edit-files-tab`} data-edit-tab="files" hidden={editTab !== "files"}>
              <label>
                {t("Folder / bucket / share")}
                <input
                  {...validationProps("path")}
                  value={connection.path}
                  onChange={(e) => change({ path: e.target.value })}
                />
              </label>
              {fieldFeedback("path")}
              <DirectoryCacheOptions wizard={wizard} />
            </div>
            <div id={`${fieldID}-edit-network-panel`} role="tabpanel" aria-labelledby={`${fieldID}-edit-network-tab`} data-edit-tab="network" hidden={editTab !== "network"}>
              <NetworkOptions wizard={wizard} />
            </div>
          </form>
        )}
    </DialogLayout>
  );
}
