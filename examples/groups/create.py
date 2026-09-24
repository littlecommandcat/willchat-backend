from config import *


_, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

_, group_body = request(
    "POST",
    "/api/groups",
    headers={"Authorization": f"Bearer {token}"},
    json={"name": "testgroup"},
)
group_id = group_body.get("data", {}).get("group", {}).get("group_id", "") if isinstance(group_body, dict) else ""