import aiohttp

from config import request

class Requester:
    def __init__(self):
        self._session: aiohttp.ClientSession | None=None
        self._data = {}
        self._token: str | None=None

    @property
    def token(self) -> str | None:
        return self._token

    def _init_session(self):
        if isinstance(self._session, aiohttp.ClientSession) and not self._session.closed:
            return self._session
        
        self._session = aiohttp.ClientSession()
        return self._session

    def _cleanup(self):
        self._session = None
        self._data.clear()

    async def close(self):
        if isinstance(self._session, aiohttp.ClientSession) and not self._session.closed:
            await self._session.close()

        self._cleanup()

    async def login(self, email: str, password: str) -> tuple[dict, str | None]:
        self._init_session()
        resp, login_body = await request(
            "POST",
            "/auth/login",
            self._session,
            json={
                "email": email,
                "password": password,
            },
        )

        if resp.status not in [200, 201]:
            return {}, None

        token = (
            login_body.get("data", {}).get("token", "")
            if isinstance(login_body, dict)
            else ""
        )
        self._data = login_body.get("data", {})
        self._token = token
        return self._data, token