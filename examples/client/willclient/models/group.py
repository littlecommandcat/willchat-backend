
class Group:
    def __init__(
        self,
        name: str,
        id: str,
        owner_id: str,
        created_at: int
    ):
        self.name: str = name
        self.id: str = id
        self.owner_id: str = owner_id
        self.created_at: int = created_at

    @classmethod
    def from_payload(cls, payload: dict) -> "Group":
        data = payload["data"]["group"] if "data" in payload else payload

        return cls(
            name=data["name"],
            id=data["group_id"],
            owner_id=data["owner_id"],
            created_at=data["created_at"],
        )
