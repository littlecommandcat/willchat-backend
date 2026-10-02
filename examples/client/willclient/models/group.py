import aiohttp

from exceptions import UnknownLogin
from cache import _cache_base
from config import BASE_URL
from ..utils import raise_not_allowed_error, is_not_allowed_code

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

    async def send_message(
        self,
        content: str,
        *,
        picture: bytes = None
    ):
        if not _cache_base.has_token():
            raise UnknownLogin(f"Client must login before send message")
        
        api = f"{BASE_URL}/api/groups/{self.id}/messages"
        headers = {"Authorization": f"{_cache_base.token}"}

        status_code = 500
        async with aiohttp.ClientSession() as session:
            if not picture:
                async with session.post(
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
                async with session.post(
                    url=api,
                    headers=headers,
                    data=data
                ) as resp:
                    status_code = resp.status

        if is_not_allowed_code(status_code):
            raise_not_allowed_error(status_code)
            return
