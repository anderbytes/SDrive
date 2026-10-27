import type { NetworkView } from "./api";
import { formatBytes } from "./formatBytes";
import { useLanguage } from "./i18n";

export function NetworkDetails({ network, mounted }: { network: NetworkView; mounted: boolean }) {
  const { t } = useLanguage();
  const included = t(network.partial
    ? "May be included in the measured total; separate value unavailable."
    : "Included in the measured total; separate value unavailable.");
  return <>
    <p>{network.startedAt ? t("Since connection: {time}", { time: new Date(network.startedAt).toLocaleString() }) : t("Network measurement is unavailable for this connection.")}</p>
    {!mounted && network.startedAt && <p>{t("Disconnected · Last connection retained until the next connection or application exit.")}</p>}
    {network.state === "stale" && <p>{t("Network measurement is outdated. The last available counters are shown.")}</p>}
    {network.partial && <p>{t("Partial coverage: some connection paths bypass the measured transport.")}</p>}
    <p>{t("Combined measured total: {size}", { size: formatBytes(network.sent === null || network.received === null ? null : network.sent + network.received) })}</p>
    <table className="network-breakdown" aria-label={t("Network usage by category")}>
      <thead><tr><th scope="col">{t("Category")}</th><th scope="col">{t("Sent")}</th><th scope="col">{t("Received")}</th></tr></thead>
      <tbody>
        <tr className="network-total"><th scope="row">{t("Measured socket total")}</th><td>{formatBytes(network.sent)}</td><td>{formatBytes(network.received)}</td></tr>
        <tr><th scope="row">{t("File contents (application)")}</th><td>{formatBytes(network.filesSent)}</td><td>{formatBytes(network.filesReceived)}</td></tr>
        {["Directory listings and metadata", "Periodic connection checks", "Authentication and token renewal", "Application retries", "TLS/SSH negotiation and overhead"].map(label =>
          <tr key={label}><th scope="row">{t(label)}</th><td colSpan={2}>{network.checkedAt ? included : t("Unavailable")}</td></tr>)}
        <tr><th scope="row">{t("TCP/IP headers, ACKs, retransmissions and system keepalive")}</th><td colSpan={2}>{t("Outside this measurement")}</td></tr>
        <tr><th scope="row">{t("System DNS and VPN overhead")}</th><td colSpan={2}>{t("Attribution unavailable")}</td></tr>
      </tbody>
    </table>
    <p>{t("File contents and socket totals measure different layers. Do not add the rows or subtract them to estimate maintenance traffic.")}</p>
    <p>{t("Counts the shared engine transport without administrator/root access. Local cache reads and the local SDrive control API are excluded. This is not an exact network-interface or ISP usage counter.")}</p>
    {network.checkedAt && <p>{t("Last network sample: {time}", { time: new Date(network.checkedAt).toLocaleTimeString() })}</p>}
  </>;
}
