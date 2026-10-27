import type { NetworkView } from "./api";
import { formatBytes } from "./formatBytes";
import { useLanguage } from "./i18n";

export function NetworkDetails({ network, mounted }: { network: NetworkView; mounted: boolean }) {
  const { t } = useLanguage();
  const rows = [
    { label: "Measured socket total", sent: network.sent, received: network.received },
    { label: "File contents (application)", sent: network.filesSent, received: network.filesReceived },
  ].filter(row => row.sent !== null || row.received !== null);
  if (!rows.length) return <p>{t("Network measurement is unavailable for this connection.")}</p>;
  return <>
    {network.startedAt && <p>{t("Since connection: {time}", { time: new Date(network.startedAt).toLocaleString() })}</p>}
    {!mounted && network.startedAt && <p>{t("Disconnected · Last connection.")}</p>}
    {network.state === "stale" && <p>{t("Outdated sample · Last counters retained.")}</p>}
    {network.partial && <p>{t("Partial network coverage.")}</p>}
    {network.sent !== null && network.received !== null && <p>{t("Combined measured total: {size}", { size: formatBytes(network.sent + network.received) })}</p>}
    <table className="network-breakdown" aria-label={t("Network usage by category")}>
      <thead><tr><th scope="col">{t("Category")}</th><th scope="col">{t("Sent")}</th><th scope="col">{t("Received")}</th></tr></thead>
      <tbody>
        {rows.map(row => <tr key={row.label} className={row.label === "Measured socket total" ? "network-total" : undefined}>
          <th scope="row">{t(row.label)}</th><td>{formatBytes(row.sent)}</td><td>{formatBytes(row.received)}</td>
        </tr>)}
      </tbody>
    </table>
    <p>{t("Approximate measurement at different layers; do not add the rows.")}</p>
    {network.checkedAt && <p>{t("Last network sample: {time}", { time: new Date(network.checkedAt).toLocaleTimeString() })}</p>}
  </>;
}
