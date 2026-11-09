from __future__ import annotations
from dataclasses import dataclass

from .group import Group


@dataclass
class Member:
    name: str
    id: str
    groups: list["Group"]
    block: list["Member"]
    read: dict[str, str]
    email: str = ""

    @classmethod
    def from_payload(cls, payload: dict) -> "Member":
        member_payload = payload.get("payload", payload)
        member_payload = member_payload.get("member", member_payload)

        groups = [
            Group.from_payload(group)
            for group in member_payload.get("groups", [])
        ]

        return cls(
            name=member_payload.get("name", ""),
            id=member_payload.get("id", ""),
            groups=groups,
            block=[cls.from_payload(item if isinstance(item, dict) else {"id": item})
                   for item in member_payload.get("block", []) or []],
            read=member_payload.get("read", {}),
            email=member_payload.get("email", ""),
        )