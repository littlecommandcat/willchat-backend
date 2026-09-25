from config import *


_, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

request("POST", f"/api/groups/{GROUP_ID}/members", headers={"Authorization": f"{token}"})