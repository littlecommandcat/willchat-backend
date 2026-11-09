from enum import Enum

class EventType(str, Enum):
    MESSAGE_SEND = "message_send"
    MESSAGE_DELETE = "message_delete"

    GROUP_CREATE = "group_create"
    GROUP_DELETE = "group_delete"
    GROUP_JOIN = "group_join"
    GROUP_LEAVE = "group_leave"
    MEMBER_UPDATE = "member_update"
    MEMBER_ONLINE = "member_online"
    MEMBER_OFFLINE = "member_offline"

    READY = "ready"
    DISCONNECT = "disconnect"
    CLOSE = "close"
    EVENT = "event"
    ERROR = "error"
    OTHER = "other"