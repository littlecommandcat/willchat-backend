from config import *


_, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

for _ in range(20):
    request("GET", f"/api/groups/{GROUP_ID}", headers={"Authorization": f"{token}"})