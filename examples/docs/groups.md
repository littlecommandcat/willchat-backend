# Backend API Docs

## create
`/groups`

Request

- POST
- JSON
    ```json
    {"name": "testgroup"}
    ```
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"group": {"group_id": "888687733403291648", "name": "testgroup", "owner_id": "888612806851563520", "created_at": "2026-09-18T07:27:46.6638523Z"}}}
    ```

## delete
`/groups/{GROUP_ID}`

Request

- DELETE
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"group_id": "888687733403291648"}}
    ```

## get
`/groups/{GROUP_ID}`

Request

- GET
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- ```json
    data: {"status": "ok", "code": "success", "message": "", "data": {"group": {"group_id": "888621284110372864", "name": "testgroup", "owner_id": "888612806851563520", "created_at": "2026-09-18T03:03:43.9178159Z"}}}
    ```

## members-join
`/groups/{GROUP_ID}/members`

Request

- POST
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- idk but you can try

## members-leave
`/groups/{GROUP_ID}/members`

Request

- DELETE
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- idk but you can try