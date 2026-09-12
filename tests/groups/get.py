import requests
import os


BASE_URL = "http://localhost:8080"
EMAIL = "testtest1@gmail.com"
PASSWORD = "123123123"
GROUP_ID = '886623179370401792'


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

group_response, group_body = request(
    "GET",
    f"/groups/{GROUP_ID}",
    headers={"Authorization": f"Bearer {token}"},
    # json={"name": "testgroup"}
)

if group_response.status_code == 200 and group_body.get("data", {}).get("group_id", group_body.get("data", {}).get("id")):
    print("get group result: success")
else:
    print("get group result: failed")