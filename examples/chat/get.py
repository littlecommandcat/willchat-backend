from config import *


AMOUNT = 20

_, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

request(
    "GET",
    f"/api/groups/{GROUP_ID}/messages",
    headers={"Authorization": f"{token}"},
    params={"amount": AMOUNT},
)