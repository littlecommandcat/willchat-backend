import os

import requests


BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
EMAIL = os.getenv("EMAIL", "testtest1@gmail.com")
PASSWORD = os.getenv("PASSWORD", "123123123")
UPDATED_USERNAME = os.getenv("UPDATED_USERNAME", "updated-test-user")


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
if login_response.status_code != 200 or not token:
    print("profile tests stopped: login failed, no token returned")
    raise SystemExit(1)

headers = {"Authorization": f"Bearer {token}"}
profile_response, profile_body = request(
    "PUT",
    "/auth/me",
    headers=headers,
    json={"username": UPDATED_USERNAME},
)
profile = profile_body.get("data", {})
if (
    profile_response.status_code != 200
    or profile.get("username") != UPDATED_USERNAME
    or not profile.get("id")
):
    print("update profile result: failed")
    raise SystemExit(1)

me_response, me_body = request("GET", "/auth/me", headers=headers)
me = me_body.get("data", {})
if me_response.status_code != 200 or me.get("username") != UPDATED_USERNAME:
    print("get updated profile result: failed")
    raise SystemExit(1)

second_login_response, second_login_body = request(
    "POST",
    "/auth/login",
    json={"email": EMAIL, "password": PASSWORD},
)
second_token = second_login_body.get("data", {}).get("token", "")
if second_login_response.status_code == 200 and second_token == token:
    print("profile result: success; login reused the same token")
else:
    print("profile result: failed; login returned a different token")
    raise SystemExit(1)