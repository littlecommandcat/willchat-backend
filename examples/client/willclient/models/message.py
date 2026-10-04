from __future__ import annotations

from dataclasses import dataclass

from .enums import MessageType

@dataclass
class Message:
    authorid: str
    content: str
    timestamp: int
    type: "MessageType"

    @classmethod
    def from_payload(cls, payload: dict) -> "Message":
        data =  payload["data"]["message"] if "data" in payload else payload
        try:
            message_type = MessageType(data["type"])
        except:
            message_type = MessageType.OTHER

        return cls(
            authorid=data["authorid"],
            content=data["content"],
            timestamp=data["timestamp"],
            type=message_type,
        )