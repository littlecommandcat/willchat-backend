import asyncio

from config import EMAIL, PASSWORD
import willclient

client = willclient.Client()


@client.listen("ready")
async def on_ready():
    print("WebSocket connected")
    print("Waiting for events; press Ctrl+C to stop")

@client.listen("event")
async def on_event(event, payload):
    print(f"On event [{event}]: {payload}")

@client.listen("message_send")
async def on_message(payload):
    print("MESSAGE:", payload)

@client.listen("message_delete")
async def on_message(payload):
    print("DELETE:", payload)

@client.listen("group_ceate")
async def on_group_join(payload):
    print("CREATE GROUP:", payload)

@client.listen("group_delete")
async def on_group_leave(payload):
    print("DELETE GROUP:", payload)

@client.listen("group_join")
async def on_group_join(payload):
    print("JOIN GROUP:", payload)

@client.listen("group_leave")
async def on_group_leave(payload):
    print("LEAVE GROUP:", payload)


@client.listen("error")
async def on_error(error):
    print("WebSocket error:", error)

@client.listen("disconnect")
async def on_disconnect(error):
    print("WebSocket disconnect:", error)


async def main():
    async with client:
        await client.start(token="MYoTLSLBc3LV5M9DLVbdRa_XnSphOGxCECn9404gL14")


try:
    asyncio.run(main())
except KeyboardInterrupt:
    # asyncio.run(client.close())
    print("WebSocket listener stopped")