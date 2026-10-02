from enum import Enum

class MessageType(Enum):
    NORMAL = 0
    REFRENCE = 1
    DEV = 2
    OTHER = 3

class Message:
    def __init__(
        self,
        authorid: str,
        content: str,
        timestamp: int,
        type: "MessageType"
    ):
        self.authorid: str = authorid
        self.content: str = content
        self.timestamp: int = timestamp
        self.type: "MessageType" = type

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