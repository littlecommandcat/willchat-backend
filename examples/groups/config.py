import os
import json
import requests


BASE_URL = "http://localhost:8080"
EMAIL = "testtest1@gmail.com"
PASSWORD = "1231231234"
USERNAME = "testtest1"
GROUP_ID = "gr-891321713063628800"
NEW_PASSWORD = "12312312345"


def request(method, path, **kwargs):
    response = requests.request(method, f"{BASE_URL}{path}", timeout=5, **kwargs)
    try:
        body = response.json()
    except ValueError:
        body = response.text
    data = body.get("data", {}) if isinstance(body, dict) else body
    print("status:", response.status_code)
    print("data:", body)
    return response, body