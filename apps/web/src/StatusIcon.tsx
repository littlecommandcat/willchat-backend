import type { ConnectionStatus } from "./lib/api";

export default function StatusIcon({ status }: { status: ConnectionStatus }) {
  const color = status === "connected" ? "green" : status === "closed" ? "red" : "yellow";
  const label = status === "connected" ? "已連線" : status === "closed" ? "未連線" : "連線中／重新連線中";
  return <div className={`api-indicator ${color}`} role="status" aria-label={`WebSocket：${label}`} title={`WebSocket：${label}`}>
    <svg width="14" height="14" viewBox="0 0 14 14"><circle cx="7" cy="7" r="6" /></svg>
  </div>;
}