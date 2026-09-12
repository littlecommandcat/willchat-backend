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
print("me login status:", login_response.status_code)
print("me login body:", login_body)
token = login_body.get("data", {}).get("token", "")
user_id = login_body.get("data", {}).get("user", {}).get("id", "")
if not token:
    print("me tests stopped: login failed, no token returned")
    raise SystemExit(1)

me_response, me_body = request(
    "GET",
    "/auth/me",
    headers={"Authorization": f"Bearer {token}"},
)
print("me status:", me_response.status_code)
print("me body:", me_body)

group_ids = me_body.get("data", {}).get("group_ids")
print("me group_ids:", group_ids)

if me_response.status_code == 200 and isinstance(group_ids, list):
    print("me result: success")
else:
    print("me result: failed")