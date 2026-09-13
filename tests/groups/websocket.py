import base64
import hashlib
import json
import os
import socket
import struct
import urllib.parse

import requests


BASE_URL = os.getenv("BASE_URL", "http://localhost:8080")
EMAIL = os.getenv("EMAIL", "testtest1@gmail.com")
PASSWORD = os.getenv("PASSWORD", "123123123")
GROUP_ID = os.getenv("GROUP_ID", "").strip()
MESSAGE_CONTENT = os.getenv("MESSAGE_CONTENT", "Hello from groups websocket test")
READ_TIMEOUT = float(os.getenv("WS_READ_TIMEOUT", "5"))


def request(method, path, **kwargs):
    response = requests.request(method, f"{BASE_URL}{path}", timeout=5, **kwargs)
    body = response.json()
    print(method, path, response.status_code, body)
    return response, body


def websocket_url(group_id, token):
    parsed = urllib.parse.urlparse(BASE_URL)
    scheme = "wss" if parsed.scheme == "https" else "ws"
    host = parsed.netloc or parsed.path
    query = urllib.parse.urlencode({"token": token})
    return f"{scheme}://{host}/ws/groups/{urllib.parse.quote(group_id, safe='')}?{query}"


def connect_websocket(url):
    parsed = urllib.parse.urlparse(url)
    host = parsed.hostname
    port = parsed.port or (443 if parsed.scheme == "wss" else 80)
    if not host:
        raise ValueError(f"invalid WebSocket URL: {url}")
    if parsed.scheme == "wss":
        raise ValueError("wss is not supported by this local test client")

    sock = socket.create_connection((host, port), timeout=READ_TIMEOUT)
    sock.settimeout(READ_TIMEOUT)
    key = base64.b64encode(os.urandom(16)).decode("ascii")
    path = parsed.path or "/"
    if parsed.query:
        path += f"?{parsed.query}"
    handshake = (
        f"GET {path} HTTP/1.1\r\n"
        f"Host: {host}:{port}\r\n"
        f"Origin: http://{host}:{port}\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        f"Sec-WebSocket-Key: {key}\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        "\r\n"
    )
    sock.sendall(handshake.encode("ascii"))
    response = read_http_response(sock)
    status_line = response.split("\r\n", 1)[0]
    if " 101 " not in f"{status_line} ":
        sock.close()
        body = response.split("\r\n\r\n", 1)[-1].strip()
        raise RuntimeError(f"WebSocket handshake failed: {status_line}: {body}")

    headers = {}
    for line in response.split("\r\n")[1:]:
        if ":" in line:
            name, value = line.split(":", 1)
            headers[name.lower().strip()] = value.strip()
    expected_accept = base64.b64encode(
        hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode("ascii")).digest()
    ).decode("ascii")
    if headers.get("sec-websocket-accept") != expected_accept:
        sock.close()
        raise RuntimeError("WebSocket handshake returned an invalid accept key")
    return sock


def read_http_response(sock):
    data = bytearray()
    while b"\r\n\r\n" not in data:
        chunk = sock.recv(4096)
        if not chunk:
            raise RuntimeError("connection closed during WebSocket handshake")
        data.extend(chunk)
        if len(data) > 65536:
            raise RuntimeError("WebSocket handshake headers are too large")
    return bytes(data).decode("latin1")


def send_text(sock, value):
    payload = value.encode("utf-8")
    mask = os.urandom(4)
    masked_payload = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))
    length = len(payload)
    if length < 126:
        header = bytes([0x81, 0x80 | length])
    elif length < 65536:
        header = bytes([0x81, 0x80 | 126]) + struct.pack("!H", length)
    else:
        header = bytes([0x81, 0x80 | 127]) + struct.pack("!Q", length)
    sock.sendall(header + mask + masked_payload)


def receive_frame(sock):
    first, second = read_exact(sock, 2)
    opcode = first & 0x0F
    masked = second & 0x80
    length = second & 0x7F
    if length == 126:
        length = struct.unpack("!H", read_exact(sock, 2))[0]
    elif length == 127:
        length = struct.unpack("!Q", read_exact(sock, 8))[0]
    mask = read_exact(sock, 4) if masked else b""
    payload = bytearray(read_exact(sock, length))
    if masked:
        for index in range(length):
            payload[index] ^= mask[index % 4]
    return opcode, bytes(payload)


def read_exact(sock, size):
    data = bytearray()
    while len(data) < size:
        chunk = sock.recv(size - len(data))
        if not chunk:
            raise RuntimeError("WebSocket connection closed")
        data.extend(chunk)
    return bytes(data)


def close_websocket(sock):
    try:
        mask = os.urandom(4)
        payload = b"\x03\xe8"
        masked_payload = bytes(byte ^ mask[index % 4] for index, byte in enumerate(payload))
        sock.sendall(bytes([0x88, 0x80 | len(payload)]) + mask + masked_payload)
    finally:
        sock.close()


login_response, login_body = request(
    "POST",
    "/auth/login",
    json={"email": EMAIL, "password": PASSWORD},
)
token = login_body.get("data", {}).get("token", "")
if not token:
    print("groups websocket tests stopped: login failed, no token returned")
    raise SystemExit(1)

if not GROUP_ID:
    group_response, group_body = request(
        "POST",
        "/groups",
        headers={"Authorization": f"Bearer {token}"},
        json={"name": "websocket-test-group"},
    )
    GROUP_ID = group_body.get("data", {}).get("group_id", "")
    if not GROUP_ID:
        GROUP_ID = group_body.get("data", {}).get("id", "")
    if group_response.status_code != 201 or not GROUP_ID:
        print("groups websocket tests stopped: could not create a test group")
        raise SystemExit(1)
    print("using newly created group_id:", GROUP_ID)
    join_response, join_body = request(
        "POST",
        f"/groups/{GROUP_ID}/members",
        headers={"Authorization": f"Bearer {token}"},
    )
    if not (
        join_response.status_code == 200
        or (
            join_response.status_code == 409
            and join_body.get("code") == "already_group_member"
        )
    ):
        print("groups websocket tests stopped: could not join the test group")
        raise SystemExit(1)
else:
    group_response, group_body = request(
        "GET",
        f"/groups/{GROUP_ID}",
        headers={"Authorization": f"Bearer {token}"},
    )
    if group_response.status_code != 200:
        print(
            "groups websocket tests stopped: GROUP_ID is not available; "
            "unset GROUP_ID to create a test group automatically"
        )
        raise SystemExit(1)

url = websocket_url(GROUP_ID, token)
print("websocket url:", url.replace(token, "<token>"))
websocket = None
try:
    websocket = connect_websocket(url)
    print("websocket connect result: success")
    event = {
        "event": "message",
        "data": {"content": MESSAGE_CONTENT},
    }
    print("websocket send:", event)
    send_text(websocket, json.dumps(event))

    message_event = None
    for _ in range(5):
        opcode, payload = receive_frame(websocket)
        if opcode == 0x9:
            continue
        if opcode == 0x8:
            raise RuntimeError("server closed WebSocket before sending message event")
        if opcode != 0x1:
            continue
        received = json.loads(payload.decode("utf-8"))
        print("websocket receive:", received)
        if received.get("event") == "message":
            message_event = received
            break

    message = (message_event or {}).get("data", {})
    if (
        message_event
        and message.get("group_id") == GROUP_ID
        and message.get("content") == MESSAGE_CONTENT
    ):
        print("groups websocket message result: success")
    else:
        print("groups websocket message result: failed")
        raise SystemExit(1)
except (OSError, RuntimeError, ValueError, json.JSONDecodeError) as error:
    print("groups websocket result: failed", error)
    raise SystemExit(1)
finally:
    if websocket is not None:
        close_websocket(websocket)