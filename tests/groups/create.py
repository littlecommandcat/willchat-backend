import requests


BASE_URL = "http://localhost:8080"
EMAIL = "testtest1@gmail.com"
PASSWORD = "123123123"


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
    "POST",
    "/groups",
    headers={"Authorization": f"Bearer {token}"},
    json={"name": "testgroup"},
)

group_id = group_body.get("data", {}).get("group_id", "")
if not group_id:
    group_id = group_body.get("data", {}).get("id", "")
print("created group_id:", group_id)
if group_response.status_code == 201 and group_id:
    print("create group result: success")
else:
    print("create group result: failed")