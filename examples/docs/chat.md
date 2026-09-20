# Backend API Docs

## get
`/groups/{GROUP_ID}/messages`

Request

- GET
- PARAMS
- ```json
    {"amount": AMOUNT}
    ```
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"amount": 20, "messages": []}}
    ```

每則訊息包含 `attachments`（只會有圖片）與選填的 `reply_to`（被回覆訊息的快照）：

```json
{
  "content": "with image",
  "attachments": [
    {"id": "889494229175242752", "mime_type": "image/jpeg", "width": 1080, "height": 648, "size": 26515, "data": "<base64 的 JPEG bytes>"}
  ],
  "reply_to": {"id": "889494225748496384", "authorid": "mr-889494223479377920", "content": "first message", "type": 0},
  "id": "889494229447872512",
  "authorid": "mr-889494223479377920",
  "timestamp": 1789908750304,
  "type": 0
}
```

`attachments[].data` 是壓縮後 JPEG 的 base64；Redis 內以原始 bytes 存在
`attachment:<attachment_id>`。

## send
`/groups/{GROUP_ID}/messages`

Request

- POST
- JSON
- ```json
    {"content": "Hello Gay", "reply_to": "被回覆的 message id", "attachments": [{"data": "base64 圖片"}]}
    ```
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

`content` 可留空，但 `content` 與 `attachments` 至少要有一個。
`reply_to` 傳入要回覆的 message id，伺服器會把該訊息快照存進新訊息。
`attachments` 只接受 JPEG / PNG / GIF，伺服器會壓縮成 JPEG（最長邊 1080、quality 80）後存成 bytes。

也可以改用 `multipart/form-data` 上傳圖片檔案：

- form fields：`content`、`reply_to`
- files：`attachments`（同名欄位可重複，最多 4 張，每張 5 MB）

Response

- 201
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"message": {"content": "Hello Gay", "id": "888633491028185088", "authorid": "mr-888612806851563520"}}}
    ```

錯誤：

- 400 `invalid_message`：content 超過 4000 字，或 content 與 attachments 都是空的
- 400 `invalid_attachment`：附件不是 JPEG / PNG / GIF
- 400 `attachment_too_large`：單張圖片超過 5 MB
- 400 `too_many_attachments`：附件超過 4 張
- 400 `reply_message_not_found`：`reply_to` 找不到對應訊息