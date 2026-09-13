import asyncio
import threading
import time
import urllib.parse

import websockets

from config import *


def websocket_url(token):
    base = BASE_URL.replace("http://", "ws://").replace("https://", "wss://")
    return f"{base}/ws?{urllib.parse.urlencode({'token': token})}"


async def listen(token):
    try:
        async with websockets.connect(websocket_url(token), origin=BASE_URL) as socket:
            print("WebSocket event listener connected")
            async for event in socket:
                print("EVENT:", event)
    except Exception as error:
        print("WebSocket event listener:", error)


_, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

listener = threading.Thread(target=lambda: asyncio.run(listen(token)), daemon=True)
listener.start()
time.sleep(1)

request(
    "POST",
    f"/groups/{GROUP_ID}/messages",
    headers={"Authorization": f"Bearer {token}"},
    json={"content": "Hello from tests/websocket/events.py"},
)
print("Waiting for WebSocket event; press Ctrl+C to stop")

try:
    while True:
        time.sleep(1)
except KeyboardInterrupt:
    print("WebSocket event test stopped")