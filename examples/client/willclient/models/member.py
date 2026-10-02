from .group import Group

class Member:
    def __init__(
        self,
        name: str,
        id: str,
        groups: list["Group"],
        block: list,
        read: dict[str, str]
    ):
        self.name: str = name
        self.id: str = id
        self.groups: list["Group"] = groups
        self.block: list = block
        self.read: dict[str, str] = read

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