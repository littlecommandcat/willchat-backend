from config import *


_, login_body = request(
	"POST",
	"/auth/login",
	json={"email": EMAIL, "password": PASSWORD},
)
token = login_body.get("data", {}).get("token", "")
request(
    "GET",
    "/auth/me",
    headers={"Authorization": f"Bearer {token}"},
)