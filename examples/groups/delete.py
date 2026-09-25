from config import *


login_response, login_body = request(
    "POST",
    "/auth/login",
    json={"email": EMAIL, "password": PASSWORD},
)
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

request(
    "DELETE",
    f"/api/groups/{GROUP_ID}",
    headers={"Authorization": f"{token}"},
)