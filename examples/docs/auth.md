# Backend API Docs

## login
`/auth/login`

Request

- POST
- JSON
- ```json
    {"email": EMAIL, "password": PASSWORD}
    ```

Response

- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"token": token, "member": {"id": "mr-888612806851563520", "username": "testtest1", "email": "testtest1@gmail.com", "group_data": [{"group_id": "gr-888621284110372864", "name": "testgroup", "owner_id": "mr-888612806851563520", "created_at": "2026-09-18T03:03:43.9178159Z"}]}}}
    ```

## register
`/auth/register`

Request

- POST
- JSON
- ```json
    {"username": USERNAME, "email": EMAIL, "password": PASSWORD}
    ```

Response

- 201
- ```json
    {
    "token": token,
        "member": {
            "id": "mr-888612806851563520",
            "username": "testtest1",
            "email": "testtest1@gmail.com",
            "group_data": []
        }
    }
    ```

## reset password
`/auth/reset`

Request

- PUT
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```
- JSON
- ```json
    {"password": NEW_PASSWORD}
    ```

Response
- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"member": {"id": "mr-888612806851563520", "username": "updated-test-user", "email": "testtest1@gmail.com", "group_data": [{"group_id": "gr-888621284110372864", "name": "testgroup", "owner_id": "mr-888612806851563520", "created_at": "2026-09-18T03:03:43.9178159Z"}]}}}
    ```

# me
`/auth/me`

Request

- GET
- JSON
- ```json
    {"Authorization": f"Bearer {token}"}
    ```

Response
- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"member": {"id": "mr-888612806851563520", "username": "testtest1", "email": "testtest1@gmail.com", "group_data": [{"group_id": "gr-888621284110372864", "name": "testgroup", "owner_id": "mr-888612806851563520", "created_at": "2026-09-18T03:03:43.9178159Z"}]}}}
    ```

## profile
`/auth/profile`

Request

- PUT
- HEADERS
- ```json
    {"Authorization": f"Bearer {token}"}
    ```
- JSON
- ```json
    {"username": UPDATED_USERNAME}
    ```

Response
- 200
- ```json
    {"status": "ok", "code": "success", "message": "", "data": {"member": {"id": "mr-888612806851563520", "username": "updated-test-user", "email": "testtest1@gmail.com", "group_data": [{"group_id": "gr-888621284110372864", "name": "testgroup", "owner_id": "mr-888612806851563520", "created_at": "2026-09-18T03:03:43.9178159Z"}]}}}
    ```