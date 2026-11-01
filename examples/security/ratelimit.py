from config import *




for _ in range(1000):
    _, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
    token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""
    # request("GET", f"/auth/me", headers={"Authorization": f"{token}"})