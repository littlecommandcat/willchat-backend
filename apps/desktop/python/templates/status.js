// A single snapshot on page entry, followed exclusively by WebSocket events.
function attachWebsocketStatus(bridge) {
    const icon = document.createElement("div");
    icon.id = "websocketIndicator";
    icon.setAttribute("role", "status");
    Object.assign(icon.style, {
        position: "fixed", top: "14px", right: "14px", width: "14px",
        height: "14px", borderRadius: "50%", border: "4px solid white",
        boxShadow: "0 1px 3px rgba(0,0,0,0.3)", zIndex: "2100",
        boxSizing: "content-box"
    });
    document.body.appendChild(icon);
    function update(status) {
        const labels = {green: "已連線", yellow: "連線中／重新連線中", red: "未連線（尚未登入或連線已關閉）"};
        icon.style.background = {green: "#2ecc40", yellow: "#ff851b", red: "#ff4136"}[status] || "#ff4136";
        icon.title = "WebSocket：" + (labels[status] || labels.red);
        icon.setAttribute("aria-label", icon.title);
    }
    update("red");
    let received = false;
    bridge.server_event.connect(function (raw) {
        let event;
        try { event = JSON.parse(raw); } catch (_) { return; }
        if (event.type === "websocket_status") {
            received = true;
            update(event.status);
        }
    });
    bridge.get_websocket_status(function (status) {
        if (!received) update(status);
    });
}