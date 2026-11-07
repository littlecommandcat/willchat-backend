export type ConnectionStatus = "closed" | "connecting" | "connected";

const labels: Record<ConnectionStatus, string> = { connected: "上線", connecting: "連線中", closed: "離線" };

export const statusLabel = (status: ConnectionStatus) => labels[status] || labels.connecting;

export default function StatusIcon({ status }: { status: ConnectionStatus }) {
  return <span className={`presence ${status}`} role="status" aria-label={`連線狀態：${statusLabel(status)}`} title={statusLabel(status)} />;
}
