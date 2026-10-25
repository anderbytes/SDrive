import type { ConnectionView, Provider } from "./api";
import { useLanguage } from "./i18n";

export function TransportCapability({ provider }: { provider: Provider }) {
  const { t } = useLanguage();
  const label = provider.transportEncryption === "required"
    ? "Traffic encryption: native and mandatory"
    : provider.transportEncryption === "optional"
      ? "Traffic encryption: available to enable"
      : provider.transportEncryption === "none"
        ? "Traffic encryption: unavailable"
        : "Traffic encryption: capability unknown";
  const help = provider.id === "smb"
    ? "SMB encryption depends on the server and share settings. Signing alone does not encrypt traffic."
    : provider.id === "mega"
      ? "MEGA transfers already encrypted file content over HTTP by default. HTTPS is an engine option; it is not currently exposed in SDrive."
      : provider.id === "ftp"
        ? "FTPS encrypts traffic using TLS. Select explicit or implicit FTPS in Advanced options."
        : provider.id === "sftp"
          ? "SFTP always encrypts traffic using SSH."
          : provider.transportEncryption === "optional"
            ? "HTTPS encrypts traffic. HTTP does not. SDrive uses HTTPS by default."
            : provider.transportEncryption === "required"
              ? "This connection type uses HTTPS to encrypt traffic."
              : "";
  return <div role="status"><p>{t(label)}</p>{help && <small>{t(help)}</small>}</div>;
}

export function TransportStatus({ connection }: { connection: ConnectionView }) {
  const { t } = useLanguage();
  if (!connection.mounted) return null;
  const security = connection.transportSecurity;
  const state = security?.state;
  const label = state === "encrypted" ? "Traffic encrypted" : state === "unencrypted" ? "Traffic unencrypted" : "Traffic encryption could not be verified";
  const evidence = security?.source === "negotiated"
    ? "Based on the negotiated SMB session and share."
    : security?.source === "protocol"
      ? "Based on the established connection and its mandatory protocol protection."
      : security?.source === "configuration"
        ? "Based on the engine's effective connection settings; not a capture of every transfer."
        : "The engine did not provide enough information to determine transport protection.";
  return <div className={state === "unencrypted" ? "inline-error" : ""}>
    <p>{t(label)}{security?.protocol && ` · ${security.protocol.toUpperCase()}`}</p>
    <small>{t(evidence)}{security?.checkedAt && <> {t("Security checked at {time}", { time: new Date(security.checkedAt).toLocaleTimeString() })}</>}</small>
    {connection.provider === "mega" && state === "unencrypted" && <p>{t("MEGA file content is already encrypted, but the transfer transport uses HTTP.")}</p>}
  </div>;
}
