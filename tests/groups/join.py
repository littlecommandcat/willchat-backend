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
user_id = login_body.get("data", {}).get("user", {}).get("id", "")
if not token:
    print("me tests stopped: login failed, no token returned")
    raise SystemExit(1)

join_response, join_body = request(
    "POST",
    f"/groups/{GROUP_ID}/members",
    headers={"Authorization": f"Bearer {token}"},
)

if join_response.status_code == 200:
    print("join group result: success")
elif join_response.status_code == 409 and join_body.get("code") == "already_group_member":
    print("join group result: already a member, no duplicate added")
else:
    print("join group result: failed")