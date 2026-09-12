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
print("logout login status:", login_response.status_code)
print("logout login body:", login_body)
token = login_body.get("data", {}).get("token", "")
if not token:
    print("logout tests stopped: login failed, no token returned")
    raise SystemExit(1)

missing_response, missing_body = request("POST", "/auth/logout")
print("logout without token status:", missing_response.status_code)
print("logout without token body:", missing_body)

logout_response, logout_body = request(
    "POST",
    "/auth/logout",
    headers={"Authorization": f"Bearer {token}"},
)
print("logout status:", logout_response.status_code)
print("logout body:", logout_body)
