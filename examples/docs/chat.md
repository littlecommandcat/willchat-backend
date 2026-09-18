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

## send
`/groups/{GROUP_ID}/messages`

Request

- POST
- JSON
- ```json
    {"content": "Hello Gay"}
    ```
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 201
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"message": {"content": "Hello Gay", "id": "888633491028185088", "authorid": 888612806851563520}}}
    ```