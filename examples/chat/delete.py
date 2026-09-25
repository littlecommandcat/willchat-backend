from config import *


msg = "ms-891212679077695488"

_, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

request(
    "DELETE",
    f"/api/groups/{GROUP_ID}/messages",
    headers={"Authorization": f"{token}"},
    params={"message_id": msg}
)