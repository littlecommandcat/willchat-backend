from __future__ import annotations
from dataclasses import dataclass

import aiohttp

from ..exceptions import UnknownLogin, UnknownSession
from ..cache import _cache_base
from config import BASE_URL
from ..utils import raise_code_error, is_not_allowed_code
from .enums import ErrorType

@dataclass
class Group:
    name: str
    id: str
    owner_id: str
    created_at: int
    owner: "Member | None" = None

    @classmethod
    def from_payload(cls, payload: dict) -> "Group":
        from .member import Member

        group_payload = payload["payload"]["group"] if "payload" in payload else payload
        owner = Member.from_payload(group_payload.get("owner") or {"id": group_payload.get("owner_id", "")})

        return cls(
            name=group_payload.get("name", ""),
            id=group_payload.get("group_id", ""),
            owner_id=owner.id,
            owner=owner,
            created_at=group_payload.get("created_at", ""),
        )

    async def send_message(
        self,
        content: str,
        *,
        picture: bytes = None
    ):
        if not _cache_base.has_token():
            raise UnknownLogin(f"Client must login before send message")

        if not _cache_base.has_session():
            raise UnknownSession(f"Client session is not found")
        
        api = f"{BASE_URL}/api/groups/{self.id}/messages"
        headers = {"Authorization": f"{_cache_base.token}"}

        status_code = 500
        if picture is None:
            async with _cache_base.session.post(
                url=api,
                headers=headers,
                json={"content": content}
            ) as resp:
                status_code = resp.status

        else:
            data = aiohttp.FormData()
            data.add_field("content", content)
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
            raise_code_error(status_code, ErrorType.GROUP)
