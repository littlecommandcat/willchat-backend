import asyncio
import os
import requests
import urllib.parse

import websockets


BASE_URL = "http://localhost:8080"
EMAIL = "testtest1@gmail.com"
PASSWORD = "123123123"


def login():
    response = requests.post(
        f"{BASE_URL}/auth/login",
        json={
            "email": EMAIL,
            "password": PASSWORD,
        },
        timeout=5,
    )

    body = response.json()

    if response.status_code != 200:
        raise RuntimeError(f"login failed: {body}")

    token = body.get("data", {}).get("token", "")

    if not token:
        raise RuntimeError("login returned no token")

    return token


def websocket_url(token):
    base = BASE_URL.replace("http://", "ws://").replace("https://", "wss://")

    return f"{base}/ws?{urllib.parse.urlencode({'token': token})}"


async def receive_events(socket):
    try:
        async for event in socket:
            print("EVENT:", event)

    except websockets.ConnectionClosed as error:
        print("WebSocket closed:", error)


async def main():
    token = login()

    async with websockets.connect(
        websocket_url(token),
        origin=BASE_URL,
    ) as socket:

        print("/ws token-authenticated connection: success")
        print("Waiting for events...")

        await receive_events(socket)


try:
    asyncio.run(main())

except KeyboardInterrupt:
    print("\nWebSocket listener stopped")

except (OSError, RuntimeError, requests.RequestException) as error:
    print("websocket connection result: failed", error)