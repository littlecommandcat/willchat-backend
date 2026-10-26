import asyncio
import json
import logging
import threading
import time

from PyQt6.QtCore import QObject, pyqtSignal, pyqtSlot, QUrl

from config import BASE_URL
from willclient.request import RequestManager
from willclient.models import Group, Member, Message
from willclient.events import EventType
from willclient.cache import _cache_base

import willclient

logger = logging.getLogger(__name__)


class Bridge(QObject):
    server_event = pyqtSignal(str)
    navigate_requested = pyqtSignal(QUrl)

    def __init__(self):
        super().__init__()
        self._loop: asyncio.AbstractEventLoop | None = None
        self._thread: threading.Thread | None = None

        self.client = willclient.Client()
        self._token = ""
        self._current_user = {"id": "", "name": ""}
        self._groups: dict = {}
        self._messages: dict = {}

        self._register_listeners()

    def start(self) -> None:
        if self._thread is not None:
            return
        self._thread = threading.Thread(target=self._run_loop, daemon=True, name="asyncio-loop")
        self._thread.start()
        while self._loop is None:
            time.sleep(0.01)

    def stop(self) -> None:
        if self._loop is not None:
            self._loop.call_soon_threadsafe(self._loop.stop)
        if self._thread is not None:
            self._thread.join(timeout=3)

    def _run_loop(self) -> None:
        self._loop = asyncio.new_event_loop()
        asyncio.set_event_loop(self._loop)
        self._loop.run_forever()

    def submit(self, coro):
        return asyncio.run_coroutine_threadsafe(coro, self._loop)

    def _request_async(self, name: str, coro, extra=None):
        fut = self.submit(coro)

        def _done(f):
            try:
                result = f.result()
            except Exception as exc:
                logger.exception("[bridge] request %s failed", name)
                result = json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False)
            payload = {"result": result}
            if extra:
                payload.update(extra)
            self._emit(name, payload)

        fut.add_done_callback(_done)
        return "pending"

    async def _check_health(self) -> tuple[bool, str]:
        try:
            _cache_base.init_session()
            return await RequestManager.health()
        except Exception:
            logger.exception("[bridge] health check failed")
            return False, "無法連線後端"
        
    @staticmethod
    def _ser_group(g: Group) -> dict:
        return {"id": g.id, "name": g.name, "owner_id": g.owner_id}

    @staticmethod
    def _ser_message(m: Message) -> dict:
        return {
            "id": m.id,
            "author_id": m.authorid,
            "content": m.content,
            "timestamp": m.timestamp,
            "type": m.type.value if hasattr(m.type, "value") else 0,
        }

    def _member_name(self, member_id: str) -> str:
        if member_id == self._current_user.get("id"):
            return self._current_user.get("name", "我")
        for group in self._groups.values():
            if member_id in group["members"]:
                return group["members"][member_id]
        return "成員"

    def _message_with_author(self, m: Message) -> dict:
        data = self._ser_message(m)
        data["author_name"] = self._member_name(m.authorid)
        return data
    
    def _register_listeners(self) -> None:
        @self.client.listen(EventType.EVENT)
        async def on_event(event_type, payload, payload_data):
            try:
                self._handle_event(event_type, payload_data)
            except Exception as exc:
                logger.exception("[bridge] event handler error for %s", event_type)
                print("[bridge] event handler error:", exc)

    def _handle_event(self, event_type, payload_data: dict) -> None:
        group = payload_data.get("group")
        message = payload_data.get("message")
        member = payload_data.get("member")

        if event_type == EventType.READY:
            self._rebuild_groups()
            self._emit("ready", {})
            return

        if event_type == EventType.MESSAGE_SEND and message is not None:
            gid = group.id if group is not None else message.group.id if message.group else ""
            if gid:
                self._messages.setdefault(gid, []).append(self._ser_message(message))
            self._emit("message_send",
                {
                    "group_id": gid,
                    "message": self._message_with_author(message),
                }
            )
            return

        if event_type == EventType.MESSAGE_DELETE and message is not None:
            gid = group.id if group is not None else ""
            self._emit("message_delete", {"group_id": gid, "message": self._ser_message(message)})
            return

        if event_type == EventType.GROUP_CREATE and group is not None:
            self._upsert_group(group)
            self._emit("group_create", {"group": self._ser_group(group)})
            return

        if event_type == EventType.GROUP_DELETE and group is not None:
            self._groups.pop(group.id, None)
            self._messages.pop(group.id, None)
            self._emit("group_delete", {"group_id": group.id})
            return

        if event_type == EventType.GROUP_JOIN and group is not None:
            self._upsert_group(group)
            if member is not None:
                self._groups[group.id]["members"][member.id] = member.name
            self._emit("group_join", {
                "group_id": group.id,
                "member": {"id": member.id if member else "", "name": member.name if member else ""},
            })
            return

        if event_type == EventType.GROUP_LEAVE and group is not None:
            if member is not None:
                self._groups.get(group.id, {}).get("members", {}).pop(member.id, None)
            self._emit("group_leave",
                {
                    "group_id": group.id,
                    "member": {"id": member.id if member else "", "name": member.name if member else ""},
                }
            )
            return

        if event_type in (EventType.ERROR, EventType.DISCONNECT):
            self._emit("error", {"type": str(event_type), "message": str(member or payload_data)})
            return

    def _upsert_group(self, group: Group) -> None:
        entry = self._groups.get(group.id)
        if entry is None:
            entry = {"id": group.id, "name": group.name, "owner_id": group.owner_id, "members": {}}
            self._groups[group.id] = entry
        else:
            entry["name"] = group.name
        uid = self._current_user.get("id")
        if uid:
            entry["members"].setdefault(uid, self._current_user.get("name", "我"))
        if group.owner_id:
            entry["members"].setdefault(group.owner_id, "群主")

    def _rebuild_groups(self) -> None:
        try:
            member: Member = self.client.member
        except Exception:
            return
        self._current_user = {"id": member.id, "name": member.name}
        self._groups = {}
        for g in member.groups:
            self._upsert_group(g)

    def _emit(self, event_type: str, data: dict) -> None:
        data["type"] = event_type
        self.server_event.emit(json.dumps(data, ensure_ascii=False))

    @pyqtSlot(str)
    def navigate(self, page: str):
        if page == "/rick":
            self.navigate_requested.emit(QUrl("https://www.youtube.com/watch?v=dQw4w9WgXcQ"))

    @pyqtSlot(str, str, result=str)
    def login(self, email: str, password: str) -> str:
        return self._request_async("login", self._async_login(email.strip(), password))

    async def _async_login(self, email: str, password: str) -> str:
        ok, msg = await self._check_health()
        if not ok:
            return json.dumps({"ok": False, "error": msg}, ensure_ascii=False)

        try:
            data, token = await RequestManager.login(email, password)
        except Exception as exc:
            logger.exception("[bridge] login request failed")
            return json.dumps({"ok": False, "error": f"無法連接伺服器"}, ensure_ascii=False)
        if not token:
            return json.dumps({"ok": False, "error": "帳號或密碼錯誤"}, ensure_ascii=False)
        self._token = token
        member = data.get("member") or {}
        self._current_user = {"id": member.get("id", ""), "name": member.get("name", "")}
        self._groups = {}
        for g in member.get("groups") or []:
            grp = Group(name=g.get("name", ""), id=g.get("group_id", ""),
                        owner_id=g.get("owner_id", ""), created_at=g.get("created_at", 0))
            self._upsert_group(grp)
        self._start_ws()
        return json.dumps({"ok": True, "member": self._current_user}, ensure_ascii=False)

    @pyqtSlot(str, str, str, result=str)
    def register(self, username: str, email: str, password: str) -> str:
        return self._request_async("register", self._async_register(username.strip(), email.strip(), password))

    async def _async_register(self, username: str, email: str, password: str) -> str:
        ok, msg = await self._check_health()
        if not ok:
            return json.dumps({"ok": False, "error": msg}, ensure_ascii=False)

        from willclient.cache import _cache_base
        _cache_base.init_session()
        try:
            import aiohttp
            async with aiohttp.ClientSession() as session:
                async with session.post(
                    f"{BASE_URL}/auth/register",
                    json={"name": username, "email": email, "password": password},
                    timeout=15,
                ) as resp:
                    text = await resp.text()
                    body = None
                    try:
                        body = json.loads(text)
                    except json.JSONDecodeError:
                        decoder = json.JSONDecoder()
                        body, _ = decoder.raw_decode(text.lstrip())
        except Exception as exc:
            logger.exception("[bridge] register request failed")
            return json.dumps({"ok": False, "error": f"無法連接伺服器"}, ensure_ascii=False)
        if resp.status not in (200, 201):
            msg = (body or {}).get("message", "註冊失敗") if isinstance(body, dict) else "註冊失敗"
            return json.dumps({"ok": False, "error": msg}, ensure_ascii=False)
        data = body.get("data") or {}
        token = data.get("token", "")
        if token:
            self._token = token
            _cache_base.token = token
            member = data.get("member") or {}
            self._current_user = {"id": member.get("id", ""), "name": member.get("name", "")}
            self._groups = {}
            for g in member.get("groups") or []:
                grp = Group(name=g.get("name", ""), id=g.get("group_id", ""),
                            owner_id=g.get("owner_id", ""), created_at=g.get("created_at", 0))
                self._upsert_group(grp)
            self._start_ws()
        return json.dumps({"ok": True}, ensure_ascii=False)

    def _start_ws(self) -> None:
        if self.client.is_running():
            return
        self.submit(self.client.start(token=self._token))

    @pyqtSlot(result=str)
    def get_state(self) -> str:
        groups = []
        for entry in self._groups.values():
            members = [{"id": mid, "name": name} for mid, name in entry["members"].items()]
            groups.append(
                {
                    "id": entry["id"],
                    "name": entry["name"],
                    "owner_id": entry["owner_id"],
                    "members": members,
                }
            )
        return json.dumps({"member": self._current_user, "groups": groups}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def get_messages(self, group_id: str) -> str:
        return self._request_async("messages", self._async_messages(group_id), {"group_id": group_id})

    async def _async_messages(self, group_id: str) -> str:
        try:
            msgs = await self.client.fetch_bulk_message(group_id, 50)
        except Exception as exc:
            logger.exception("[bridge] fetch messages failed for group %s", group_id)
            return json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False)
        data = [self._message_with_author(m) for m in reversed(msgs)]
        self._messages[group_id] = data
        return json.dumps({"ok": True, "messages": data}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def get_members(self, group_id: str) -> str:
        entry = self._groups.get(group_id)
        members = []
        if entry:
            members = [{"id": mid, "name": name} for mid, name in entry["members"].items()]
        return json.dumps({"members": members}, ensure_ascii=False)

    @pyqtSlot(str, str, result=str)
    def send_message(self, group_id: str, content: str) -> str:
        return self._request_async("message_sent", self._async_send(group_id, content.strip()), {"group_id": group_id})

    async def _async_send(self, group_id: str, content: str) -> str:
        if not content:
            return json.dumps({"ok": False, "error": "訊息不能為空"}, ensure_ascii=False)
        entry = self._groups.get(group_id)
        group = Group(
            name=entry["name"] if entry else "",
            id=group_id,
            owner_id=entry["owner_id"] if entry else "",
            created_at=0
        )
        try:
            await group.send_message(content)
        except Exception as exc:
            logger.exception("[bridge] send message failed for group %s", group_id)
            return json.dumps({"ok": False, "error": f"傳送失敗：{exc}"}, ensure_ascii=False)
        return json.dumps({"ok": True}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def create_group(self, name: str) -> str:
        return self._request_async("group_created", self._async_create_group(name.strip()), {"group_id": ""})

    async def _async_create_group(self, name: str) -> str:
        if not name:
            return json.dumps({"ok": False, "error": "群組名稱不能為空"}, ensure_ascii=False)
        try:
            group = await self.client.create_group(name)
        except Exception as exc:
            logger.exception("[bridge] create group failed")
            return json.dumps({"ok": False, "error": f"建立群組失敗：{exc}"}, ensure_ascii=False)
        self._upsert_group(group)
        return json.dumps({"ok": True, "group": self._ser_group(group)}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def delete_group(self, group_id: str) -> str:
        return self._request_async("group_deleted", self._async_delete_group(group_id), {"group_id": group_id})

    async def _async_delete_group(self, group_id: str) -> str:
        try:
            await self.client.delete_group(group_id)
        except Exception as exc:
            logger.exception("[bridge] delete group failed for group %s", group_id)
            return json.dumps({"ok": False, "error": f"刪除群組失敗：{exc}"}, ensure_ascii=False)
        self._groups.pop(group_id, None)
        self._messages.pop(group_id, None)
        return json.dumps({"ok": True}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def join_group(self, group_id: str) -> str:
        return self._request_async("group_joined", self._async_join_group(group_id), {"group_id": group_id})

    async def _async_join_group(self, group_id: str) -> str:
        try:
            group = await self.client.join_group(group_id)
        except Exception as exc:
            logger.exception("[bridge] join group failed for group %s", group_id)
            return json.dumps({"ok": False, "error": f"加入群組失敗：{exc}"}, ensure_ascii=False)
        self._upsert_group(group)
        return json.dumps({"ok": True, "group": self._ser_group(group)}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def leave_group(self, group_id: str) -> str:
        return self._request_async("group_left", self._async_leave_group(group_id), {"group_id": group_id})

    async def _async_leave_group(self, group_id: str) -> str:
        try:
            await self.client.leave_group(group_id)
        except Exception as exc:
            logger.exception("[bridge] leave group failed for group %s", group_id)
            return json.dumps({"ok": False, "error": f"退出群組失敗：{exc}"}, ensure_ascii=False)
        self._groups.pop(group_id, None)
        self._messages.pop(group_id, None)
        return json.dumps({"ok": True}, ensure_ascii=False)

