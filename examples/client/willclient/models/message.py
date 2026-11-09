from __future__ import annotations
from dataclasses import dataclass, field

import aiohttp

from ..exceptions import UnknownLogin, UnknownSession, UnknownMessage
from ..cache import _cache_base
from config import BASE_URL
from ..utils import raise_code_error, is_not_allowed_code
from .enums import MessageType, ErrorType
from .group import Group
from .member import Member


@dataclass
class ReplyTo:
    id: str
    author: Member
    content: str
    type: int

    @classmethod
    def from_payload(cls, payload: dict) -> "ReplyTo":
        return cls(id=payload.get("id", ""),
                   author=Member.from_payload(payload.get("author") or {"id": payload.get("authorid", "")}),
                   content=payload.get("content", ""), type=payload.get("type", 0))


@dataclass
class Message:
    author: Member
    id: str
    content: str
    timestamp: int
    type: "MessageType"
    group: "Group" | None
    attachments: list = field(default_factory=list)
    reply_to: ReplyTo | None = None
    status: str = ""

    @property
    def authorid(self) -> str:
        return self.author.id

    @classmethod
    def from_payload(cls, payload: dict) -> "Message":
        message_payload =  payload["payload"]["message"] if "payload" in payload else payload
        group_payload = payload["payload"]["group"] if "payload" in payload and "group" in payload.get("payload", {}) else {}

        group = None
        if group_payload:
            group = Group.from_payload(group_payload)

        try:
            message_type = MessageType(message_payload["type"])
        except (ValueError, KeyError):
            message_type = MessageType.OTHER

        return cls(
            author=Member.from_payload(message_payload.get("author") or {"id": message_payload.get("authorid", "")}),
            id=message_payload.get("id", ""),
            content=message_payload.get("content", ""),
            timestamp=message_payload.get("timestamp", -1),
            type=message_type,
            group=group,
            attachments=message_payload.get("attachments", []) or [],
            reply_to=ReplyTo.from_payload(message_payload["reply_to"]) if message_payload.get("reply_to") else None,
            status=message_payload.get("status", ""),
        )

    async def reply_message(
        self,
        content: str,
        *,
        picture: bytes = None
    ):
        if not _cache_base.has_token():
            raise UnknownLogin(f"Client must login before send message")

        if not _cache_base.has_session():
            raise UnknownSession(f"Client session is not found")

        if not self.id or not self.group:
            raise UnknownMessage(f"Cannot found the message to reply")
        
        api = f"{BASE_URL}/api/groups/{self.group.id}/messages"
        headers = {"Authorization": f"{_cache_base.token}"}

        status_code = 500
        if picture is None:
            async with _cache_base.session.post(
                url=api,
                headers=headers,
                json={"content": content, "reply_to": self.id}
            ) as resp:
                status_code = resp.status

        else:
            data = aiohttp.FormData()
            data.add_field("content", content)
            data.add_field("reply_to", self.id)
            data.add_field(
                "attachments",
                picture, 
                filename="sample.png", 
                content_type="image/png"
            )
            async with _cache_base.session.post(
                url=api,
                headers=headers,
                data=data
            ) as resp:
                status_code = resp.status

        if is_not_allowed_code(status_code):
            raise_code_error(status_code, ErrorType.MESSAGE)
