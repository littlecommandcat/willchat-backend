# Backend API Docs

## me
`/api/me`

Request

- GET
- JSON
- ```json
    {"Authorization": f"member {token}"}
    ```

Response
- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"member": {"id": "mr-888612806851563520", "username": "testtest1", "email": "testtest1@gmail.com", "groups": [{"group_id": "gr-888621284110372864", "name": "testgroup", "owner_id": "mr-888612806851563520", "created_at": "1789909477240"}]}}}
    ```

## profile
`/api/me`

Request

- PUT
- HEADERS
- ```json
    {"Authorization": f"member {token}"}
    ```
- JSON
- ```json
    {"username": UPDATED_USERNAME}
    ```

Response
- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"member": {"id": "mr-888612806851563520", "username": "updated-test-user", "email": "testtest1@gmail.com", "groups": [{"group_id": "gr-888621284110372864", "name": "testgroup", "owner_id": "mr-888612806851563520", "created_at": "1789909477240"}]}}}
    ```