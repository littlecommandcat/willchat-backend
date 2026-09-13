import os
import requests


BASE_URL = "http://localhost:8080"
EMAIL = "testtest1@gmail.com"
PASSWORD = "1231231234"
USERNAME = "testtest1"
GROUP_ID = "886949059682635776"
NEW_PASSWORD = "12312312345"


def request(method, path, **kwargs):
    response = requests.request(method, f"{BASE_URL}{path}", timeout=5, **kwargs)
    try:
        body = response.json()
    except ValueError:
        body = response.text
    print(method, path, response.status_code, body)
    return response, body