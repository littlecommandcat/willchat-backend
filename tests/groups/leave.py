import os

import requests


BASE_URL = "http://localhost:8080"
EMAIL = "testtest1@gmail.com"
PASSWORD = "123123123"
GROUP_ID = os.getenv("GROUP_ID", "886623179370401792")


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
token = login_body.get("data", {}).get("token", "")
if not token:
    print("leave group tests stopped: login failed, no token returned")
    raise SystemExit(1)

leave_response, leave_body = request(
    "DELETE",
    f"/groups/{GROUP_ID}/members",
    headers={"Authorization": f"Bearer {token}"},
)
print("leave group body:", leave_body)

if leave_response.status_code == 200:
    print("leave group result: success")
else:
    print("leave group result: failed")