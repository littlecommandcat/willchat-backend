import os
import json
import aiohttp

from typing import Any


BASE_URL = "http://localhost:8080"
EMAIL = "testtest1@gmail.com"
PASSWORD = "1231231234"
USERNAME = "testtest1"
GROUP_ID = "gr-889496926997712896"
NEW_PASSWORD = "12312312345"


async def request(method, path, session: aiohttp.ClientSession, **kwargs) -> tuple[aiohttp.ClientResponse, Any]:
    response = await session.request(method, f"{BASE_URL}{path}", timeout=5, **kwargs)
    try:
        body = await response.json()
    except ValueError:
        body = await response.text
    data = body.get("data", {}) if isinstance(body, dict) else body
    print("status:", response.status)
    print("data:", body)
    return response, body