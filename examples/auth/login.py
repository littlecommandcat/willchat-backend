from config import *


request(
    "POST",
    "/auth/login",
    json={"email": EMAIL, "password": PASSWORD},
)