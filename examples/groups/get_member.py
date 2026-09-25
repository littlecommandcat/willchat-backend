from config import *


_, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
login_data = login_body.get("data", {}) if isinstance(login_body, dict) else {}
token = login_data.get("token", "")
member_id = login_data.get("member", {}).get("id", "")

request("GET", f"/api/groups/{GROUP_ID}/members/{member_id}", headers={"Authorization": f"{token}"})
