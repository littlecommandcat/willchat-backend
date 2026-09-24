from config import *


UPDATED_USERNAME = "updated-test-user"


_, login_body = request(
    "POST",
    "/auth/login",
    json={"email": EMAIL, "password": PASSWORD},
)
token = login_body.get("data", {}).get("token", "")

headers = {"Authorization": f"member {token}"}
request(
    "PUT",
    "/api/me",
    headers=headers,
    json={"username": UPDATED_USERNAME},
)