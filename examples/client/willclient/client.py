import asyncio
import json
import urllib.parse
from collections import defaultdict

import websockets

from config import request, EMAIL, PASSWORD, BASE_URL
from .request import Requester
from .objects import Member, Message, Group

class Client:
    def __init__(self):
        self._listeners = defaultdict(list)
        self._socket = None
        self._running = False
        self._requester = Requester()

    async def __aenter__(self):
        return self

    async def __aexit__(self, exc_type, exc_value, traceback):
        await self.close()

    # def is_running(self) -> bool:
    #     return self._running
    
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
        self._requester._init_session()
        self._running = True
        if email and password:
            data, token = await self._requester.login(email, password)
            # print(data, token)

        if not token:
            self._running = False
            raise RuntimeError("Login failed")

        while self._running:
            try:
                async with websockets.connect(
                    self.websocket_url(token),
                    origin=BASE_URL,
                    ping_interval=None
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
                        event_type: str | None = event.get("event")
                        payload = event.get("payload")

                        if not event_type:
                            continue

                        payload_data = payload
                        # if event_type.startswith("message"):
                        #     payload_data = Message.from_payload(payload)
                        # elif event_type.startswith("group"):
                        #     payload_data = Group.from_payload(payload)
                        # elif event_type.startswith("member"):
                        #     payload_data = Member.from_payload(payload)
                        
                        await self._dispatch(event_type, payload_data)
                        await self._dispatch("event", event_type, payload_data)

            except asyncio.CancelledError:
                raise

            except websockets.ConnectionClosed as error:
                await self._dispatch("disconnect", error)

                if self._running:
                    await asyncio.sleep(3)

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

        if not self._requester.closed:
            await self._requester.close()


    async def get_group(self, group_id: str) -> None: ...