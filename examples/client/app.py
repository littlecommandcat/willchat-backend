import asyncio

from config import EMAIL, PASSWORD
import willclient
from willclient import Message, Group, Member, EventType, commands

client = willclient.Client()


@client.listen("ready")
async def on_ready():
    print("WebSocket connected")
    print("Waiting for events; press Ctrl+C to stop")

# @client.listen("event")
# async def on_event(event: EventType, payload, **kwargs):
#     print(f"On event [{event}]: {payload} -> {kwargs}")

# @client.listen("message_send")
# async def on_message(message: Message):
#     print("MESSAGE:", message)

@client.listen("message_delete")
async def on_message(message: Message):
    print("DELETE:", message)

@client.listen("group_ceate")
async def on_group_join(group: Group, member: Member):
    print("CREATE GROUP:", group, member)

@client.listen("group_delete")
async def on_group_leave(group: Group, member: Member):
    print("DELETE GROUP:", group, member)

@client.listen("group_join")
async def on_group_join(group: Group, member: Member):
    print("JOIN GROUP:", group, member)

@client.listen("group_leave")
async def on_group_leave(group: Group, member: Member):
    print("LEAVE GROUP:", group, member)


@client.listen("error")
async def on_error(error):
    print("WebSocket error:", error)

@client.listen("disconnect")
async def on_disconnect(error):
    print("WebSocket disconnect:", error)

@client.command(name="echo")
async def echo_command(cda: commands.CommandData, a: str, b: str="test"):
    print("=" * 40)
    print(f"""Command '{cda.cmd.name}' from {cda.group} attaches <{a}, {b}>
Original message {cda.message}
""")
    print("=" * 40)

async def main():
    async with client:
        await client.start(EMAIL, PASSWORD)


try:
    asyncio.run(main())
except KeyboardInterrupt:
    # asyncio.run(client.close())
    print("WebSocket listener stopped")