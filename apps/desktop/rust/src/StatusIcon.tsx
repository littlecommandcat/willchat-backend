import { useEffect, useState } from "react";
import { api, onServerEvent } from "./lib/api";

export default function StatusIcon() {
  const [status, setStatus] = useState("closed");
  useEffect(() => {
    let cancelled = false;
    let stop: (() => void) | undefined;
    let received = false;
    void (async () => {
      const unlisten = await onServerEvent((event) => {
        if (!cancelled && event.type === "websocket_status") {
          received = true;
          setStatus(String(event.status));
        }
      });
      if (cancelled) { unlisten(); return; }
      stop = unlisten;
      const current = await api.getWebsocketStatus();
      if (!cancelled && !received) setStatus(current);
    })().catch(() => { if (!cancelled) setStatus("closed"); });
    return () => { cancelled = true; stop?.(); };
  }, []);
  const color = status === "connected" ? "green" : status === "closed" ? "red" : "yellow";
  const label = status === "connected" ? "已連線" : status === "closed" ? "未連線（尚未登入或連線已關閉）" : "連線中／重新連線中";
  return <div className={`api-indicator ${color}`} role="status" aria-label={`WebSocket：${label}`} title={`WebSocket：${label}`}>
    <svg width="14" height="14" viewBox="0 0 14 14"><circle cx="7" cy="7" r="6" /></svg>
  </div>;
}