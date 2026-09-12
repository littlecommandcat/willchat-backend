import requests


BASE_URL = "http://localhost:8080"
EMAIL = "testtest1@gmail.com"
PASSWORD = "123123123"


def request(method, path, **kwargs):
    response = requests.request(method, f"{BASE_URL}{path}", timeout=5, **kwargs)
    body = response.json()
    print(method, path, response.status_code, body)
    return response, body


response, body = request(
    "POST",
    "/auth/register",
    json={"email": EMAIL, "password": PASSWORD},
)
print("register status:", response.status_code)
print("register body:", body)

if response.status_code == 201:
    print("register result: registered successfully")
elif response.status_code == 409:
    print("register result: account already exists; password was not changed")
else:
    print("register result: registration failed")