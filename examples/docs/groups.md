# Backend API Docs

## create
`/api/groups`

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
`/api/groups/{GROUP_ID}`

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
`/api/groups/{GROUP_ID}`

Request

- GET
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- ```json
    data: {"status": "ok", "code": "success", "message": "", "data": {"group": {"group_id": "gr-889496926997712896", "name": "testgroup", "owner_id": "888612806851563520", "created_at": "1789909477240"}}}
    ```

## members-join
`/api/groups/{GROUP_ID}/members`

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
`/api/groups/{GROUP_ID}/members`

Request

- DELETE
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- idk but you can try

## member-get
`/api/groups/{GROUP_ID}/members/{MEMBER_ID}`

Request

- GET
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response

- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"group_id": "gr-888687733403291648", "member": {"id": "mr-888612806851563520", "username": "testtest1"}}}
    ```