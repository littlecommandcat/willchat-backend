import asyncio
import json
import urllib.parse
from collections import defaultdict

import websockets

from config import request, EMAIL, PASSWORD, BASE_URL
from .request import Requester

class Client:
    def __init__(self):
        self._listeners = defaultdict(list)
        self._socket = None
        self._running = False
        self._requester = Requester()

    def listen(self, event):
        def decorator(func):
            self._listeners[event].append(func)
            return func

        return decorator

    async def _dispatch(self, event, *args):
        for callback in self._listeners.get(event, []):
            result = callback(*args)

            if asyncio.iscoroutine(result):
                await result

    def websocket_url(self, token):
        base = (
            BASE_URL
            .replace("http://", "ws://")
            .replace("https://", "wss://")
        )

        query = urllib.parse.urlencode({
            "token": token
        })

        return f"{base}/ws?{query}"

    async def start(self, email: str=None, password: str=None, *, token: str=None):
        
        self._running = True
        if email and password:
            data, token = await self._requester.login(email, password)
            print(data, token)

        if not token:
            self._running = False
            raise RuntimeError("Login failed")

        while self._running:
            try:
                async with websockets.connect(
                    self.websocket_url(token),
                    origin=BASE_URL,
                ) as socket:
                    self._socket = socket

                    await self._dispatch("ready")

                    async for raw_event in socket:
                        try:
                            event = json.loads(raw_event)
                        except json.JSONDecodeError:
                            print("Invalid WebSocket event:", raw_event)
                            continue

                        # print(event)
                        event_type = event.get("event")
                        payload = event.get("payload")

                        if not event_type:
                            continue

                        await self._dispatch(event_type, payload)
                        await self._dispatch("event", event_type, payload)

            except asyncio.CancelledError:
                raise

            except Exception as error:
                await self._dispatch("error", error)

                if self._running:
                    await asyncio.sleep(3)

            finally:
                self._socket = None

    async def close(self):
        self._running = False

        if self._socket is not None:
            await self._socket.close()