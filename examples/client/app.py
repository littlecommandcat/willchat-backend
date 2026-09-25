import asyncio

from config import EMAIL, PASSWORD
import willclient
from willclient import Message, Group, Member

client = willclient.Client()


@client.listen("ready")
async def on_ready():
    print("WebSocket connected")
    print("Waiting for events; press Ctrl+C to stop")

@client.listen("event")
async def on_event(event, payload):
    print(f"On event [{event}]: {payload}")

@client.listen("message_send")
async def on_message(message: Message):
    print("MESSAGE:", message)

@client.listen("message_delete")
async def on_message(message: Message):
    print("DELETE:", message)

@client.listen("group_ceate")
async def on_group_join(group: Group):
    print("CREATE GROUP:", group)

@client.listen("group_delete")
async def on_group_leave(group: Group):
    print("DELETE GROUP:", group)

@client.listen("group_join")
async def on_group_join(group: Group):
    print("JOIN GROUP:", group)

@client.listen("group_leave")
async def on_group_leave(group: Group):
    print("LEAVE GROUP:", group)


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