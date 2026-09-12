import os

import requests


BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
EMAIL = os.getenv("EMAIL", "testtest1@gmail.com")
PASSWORD = os.getenv("PASSWORD", "123123123")
GROUP_ID = os.getenv("GROUP_ID", "886623179370401792")
MESSAGE_CONTENT = os.getenv("MESSAGE_CONTENT", "Hello from tests/chat/send.py")


def request(method, path, **kwargs):
    response = requests.request(method, f"{BASE_URL}{path}", timeout=5, **kwargs)
    body = response.json()
    print(method, path, response.status_code, body)
    return response, body


login_response, login_body = request(
    "POST",
    "/auth/login",
    json={"email": EMAIL, "password": PASSWORD},
)
print("chat login status:", login_response.status_code)
token = login_body.get("data", {}).get("token", "")
if not token:
    print("send message tests stopped: login failed, no token returned")
    raise SystemExit(1)

message_fields = {
    "content": MESSAGE_CONTENT,
}
print("message fields:", message_fields)

message_response, message_body = request(
    "POST",
    f"/groups/{GROUP_ID}/messages",
    headers={"Authorization": f"Bearer {token}"},
    json=message_fields,
)
print("message response:", message_body)

message = message_body.get("data", {})
if (
    message_response.status_code == 201
    and message.get("group_id") == GROUP_ID
    and message.get("content") == MESSAGE_CONTENT
):
    print("send message result: success")
else:
    print("send message result: failed")