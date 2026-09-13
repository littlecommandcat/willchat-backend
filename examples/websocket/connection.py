import asyncio
import urllib.parse

import websockets

from config import *


def websocket_url(token):
    base = BASE_URL.replace("http://", "ws://").replace("https://", "wss://")
    return f"{base}/ws?{urllib.parse.urlencode({'token': token})}"


async def main():
    _, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
    token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""

    try:
        async with websockets.connect(websocket_url(token), origin=BASE_URL) as socket:
            print("WebSocket connected")
            print("Waiting for events; press Ctrl+C to stop")
            async for event in socket:
                print("EVENT:", event)
    except Exception as error:
        print("WebSocket result:", error)


try:
    asyncio.run(main())
except KeyboardInterrupt:
    print("WebSocket listener stopped")