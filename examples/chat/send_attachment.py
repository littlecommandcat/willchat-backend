import struct
import zlib

from config import *


def sample_png(width=1600, height=1200):
    """Build a small gradient PNG in memory so the example runs standalone."""
    raw = bytearray()
    for y in range(height):
        raw.append(0)
        for x in range(width):
            raw += bytes(((x * 255) // width, (y * 255) // height, 128))

    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)

    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
        + chunk(b"IDAT", zlib.compress(bytes(raw), 9))
        + chunk(b"IEND", b"")
    )


_, login_body = request("POST", "/auth/login", json={"email": EMAIL, "password": PASSWORD})
token = login_body.get("data", {}).get("token", "") if isinstance(login_body, dict) else ""
headers = {"Authorization": f"Bearer {token}"}

_, first_body = request("POST", f"/groups/{GROUP_ID}/messages", headers=headers, json={"content": "Hello Gay"})
reply_to = first_body.get("data", {}).get("message", {}).get("id", "") if isinstance(first_body, dict) else ""

# Images only: JPEG/PNG/GIF, at most 4 files, 5 MB each. The server compresses
# every image to JPEG (longest edge 1080, quality 80) before storing the bytes.
request(
    "POST",
    f"/api/groups/{GROUP_ID}/messages",
    headers=headers,
    data={"content": "with image", "reply_to": reply_to},
    files={"attachments": ("sample.png", sample_png(), "image/png")},
)