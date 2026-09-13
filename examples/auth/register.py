from config import *


response, body = request(
    "POST",
    "/auth/register",
    json={"username": USERNAME, "email": EMAIL, "password": PASSWORD},
)