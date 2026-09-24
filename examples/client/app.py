import asyncio

from config import EMAIL, PASSWORD
import willclient

client = willclient.Client()


@client.listen("ready")
async def on_ready():
    print("WebSocket connected")
    print("Waiting for events; press Ctrl+C to stop")

# @client.listen("event")
# async def on_event(event, payload):
#     print(f"On event [{event}]: {payload}")

@client.listen("message_send")
async def on_message(payload):
    print("MESSAGE:", payload.get("data", {}))


@client.listen("group_join")
async def on_group_join(payload):
    print("JOIN GROUP:", payload.get("data", {}))


@client.listen("group_leave")
async def on_group_leave(payload):
    print("LEAVE GROUP:", payload.get("data", {}))


@client.listen("error")
async def on_error(error):
    print("WebSocket error:", error)

@client.listen("disconnect")
async def on_disconnect(error):
    print("WebSocket disconnect:", error)


async def main():
    async with client:
        await client.start(EMAIL, PASSWORD)


try:
    asyncio.run(main())
except KeyboardInterrupt:
    # asyncio.run(client.close())
    print("WebSocket listener stopped")