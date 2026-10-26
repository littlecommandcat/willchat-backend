from config import *


_, login_body = request("POST", "/auth/login", json={"email": "123@gmail.com", "password": "12345678"})
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

request("POST", f"/api/groups/gr-902534547076288512/members", headers={"Authorization": f"{token}"})