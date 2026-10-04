from __future__ import annotations
from typing import TYPE_CHECKING
from dataclasses import dataclass

if TYPE_CHECKING:
    from .group import Group

@dataclass
class Member:
    name: str
    id: str = id
    groups: list["Group"]
    block: list
    read: dict[str, str]

    @classmethod
    def from_payload(cls, payload: dict) -> "Member":
        data = payload["data"]["member"] if "data" in payload else payload

        groups = [
            Group(
                name=group.get("name", ""),
                group_id=group.get("group_id", ""),
                owner_id=group.get("owner_id", ""),
                created_at=group.get("created_at", ""),
            )
            for group in data.get("groups", [])
        ]

        return cls(
            name=data["name"],
            id=data["id"],
            groups=groups,
            block=data.get("block", []),
            read=data.get("read", {}),
        )