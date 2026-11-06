# WillChat Web

使用 Node.js 18 以上版本。在儲存庫根目錄執行：

```sh
npm --prefix apps/web ci
npm --prefix apps/web start
```

開啟 `http://localhost:3000`。開發伺服器會將 API 與 WebSocket 請求轉發到 `http://localhost:8080`，請先啟動 Go 後端。

正式部署使用 `npm --prefix apps/web run build`。由同一網域提供 `build` 靜態檔案，並將 `/api`、`/auth` 和 `/ws` 轉發至後端；`/ws` 需要支援 WebSocket 升級。如果使用獨立 API 網域，建置前設定 `REACT_APP_API_URL`，並在 API 閘道允許前端來源及 `Authorization`、`Content-Type` 標頭。

目前後端沒有提供完整群組名單的端點，因此成員欄只列出已知成員。