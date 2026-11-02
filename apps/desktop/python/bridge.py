import asyncio
import json
import logging
import threading
import time

from PyQt6.QtCore import QObject, pyqtSignal, pyqtSlot, QUrl
from PyQt6.QtGui import QGuiApplication

from config import BASE_URL
from willclient.request import RequestManager, REQUEST_TIMEOUT_SECONDS
from willclient.models import Group, Member, Message
from willclient.events import EventType
from willclient.cache import _cache_base

import willclient as willclient

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
        self._ws_status = "red"
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
                code = getattr(exc, "code", "") or ""
                error = getattr(exc, "message", "") or str(exc)
                result = json.dumps({"ok": False, "error": error, "code": code}, ensure_ascii=False)
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

    @pyqtSlot(result=str)
    def health(self) -> str:
        fut = self.submit(self._check_health())

        def _done(f):
            try:
                ok, msg = f.result()
            except Exception:
                logger.exception("[bridge] health check failed")
                ok, msg = False, "無法連線後端"
            self._emit("health", {"ok": ok, "error": msg})

        fut.add_done_callback(_done)
        return "pending"

    @pyqtSlot(str, result=str)
    def copy_to_clipboard(self, text: str) -> str:
        try:
            clipboard = QGuiApplication.clipboard()
            if clipboard is None:
                return json.dumps({"ok": False, "error": "無法存取剪貼簿"}, ensure_ascii=False)
            clipboard.setText(text)
            return json.dumps({"ok": True}, ensure_ascii=False)
        except Exception as exc:
            logger.exception("[bridge] copy to clipboard failed")
            return json.dumps({"ok": False, "error": f"複製失敗：{exc}"}, ensure_ascii=False)


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
            "attachments": m.attachments or [],
            "reply_to": m.reply_to,
            "status": m.status,
            "group": {"id": m.group.id, "name": m.group.name} if m.group else None,
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
            event_name = event_type.value if hasattr(event_type, "value") else str(event_type)
            self._emit("server_event", {"event": event_name, "payload": payload})
            if event_type == EventType.READY:
                self._emit("websocket_status", {"status": "green"})
            try:
                self._handle_event(event_type, payload_data)
            except Exception as exc:
                logger.exception("[bridge] event handler error for %s", event_type)
                print("[bridge] event handler error:", exc)

        for lifecycle_event in (EventType.ERROR, EventType.DISCONNECT, EventType.CLOSE):
            @self.client.listen(lifecycle_event)
            async def on_lifecycle_event(*args, event_type=lifecycle_event):
                event_name = event_type.value
                message = str(args[0]) if args else ""
                status = {
                    EventType.ERROR: "yellow",
                    EventType.DISCONNECT: "yellow",
                    EventType.CLOSE: "red",
                }[event_type]
                self._emit("websocket_status", {"status": status, "message": message})
                self._emit("server_event", {
                    "event": event_name,
                    "payload": {"message": message},
                })
                if event_type in (EventType.ERROR, EventType.DISCONNECT):
                    self._emit("error", {"type": event_name, "message": message})

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

        if event_type == EventType.MEMBER_UPDATE and member is not None:
            if member.id == self._current_user.get("id"):
                self._current_user["name"] = member.name
            for entry in self._groups.values():
                if member.id in entry["members"]:
                    entry["members"][member.id] = member.name
            self._emit("member_update", {
                "member": {"id": member.id, "name": member.name},
            })
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
        if event_type == "websocket_status":
            self._ws_status = data["status"]
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
        _cache_base.init_session()
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
        from willclient.cache import _cache_base
        _cache_base.init_session()
        try:
            import aiohttp
            async with aiohttp.ClientSession() as session:
                async with session.post(
                    f"{BASE_URL}/auth/register",
                    json={"name": username, "email": email, "password": password},
                    timeout=REQUEST_TIMEOUT_SECONDS,
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
        self._emit("websocket_status", {"status": "yellow"})
        self.submit(self.client.start(token=self._token))

    @pyqtSlot(result=str)
    def get_websocket_status(self) -> str:
        return self._ws_status

    @pyqtSlot(str, result=str)
    def update_profile(self, name: str) -> str:
        if len(name.strip()) < 3:
            return json.dumps({"ok": False, "error": "使用者名稱須超過3字元"}, ensure_ascii=False)

        return self._request_async("profile_updated", self._async_update_profile(name.strip()))

    async def _async_update_profile(self, name: str) -> str:
        try:
            profile = {"name": name}
            resp, body = await RequestManager.put("/api/me", headers={"Authorization": RequestManager.token}, json=profile)
            if resp.status not in (200, 201):
                return json.dumps({"ok": False, "code": (body or {}).get("code", ""), "error": (body or {}).get("message", "更新個人資料失敗")}, ensure_ascii=False)
            member = (body.get("data") or {}).get("member") or {}
            self._current_user.update({"id": member.get("id", self._current_user["id"]), "name": member.get("name", name)})
            return json.dumps({"ok": True, "member": self._current_user}, ensure_ascii=False)
        except Exception as exc:
            return json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def reset_password(self, password: str) -> str:
        if len(password) < 8:
            return json.dumps({"ok": False, "error": "密碼必須超過8字元"}, ensure_ascii=False)
        return self._request_async("password_reset", self._async_reset_password(password))

    async def _async_reset_password(self, password: str) -> str:
        try:
            resp, body = await RequestManager.post("/auth/reset", headers={"Authorization": RequestManager.token}, json={"password": password})
            if resp.status not in (200, 201, 202, 204):
                return json.dumps({"ok": False, "code": (body or {}).get("code", ""), "error": (body or {}).get("message", "重設密碼失敗")}, ensure_ascii=False)
            await self.client.close()
            _cache_base.token = ""
            self._token = ""
            self._current_user = {"id": "", "name": ""}
            self._groups.clear()
            self._messages.clear()
            self._emit("websocket_status", {"status": "red"})
            self._emit("auth_required", {})
            return json.dumps({"ok": True}, ensure_ascii=False)
        except Exception as exc:
            return json.dumps({"ok": False, "error": str(exc)}, ensure_ascii=False)

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
            code = getattr(exc, "code", "") or ""
            error = getattr(exc, "message", "") or str(exc)
            return json.dumps({"ok": False, "error": f"傳送失敗：{error}", "code": code}, ensure_ascii=False)
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
            code = getattr(exc, "code", "") or ""
            error = getattr(exc, "message", "") or str(exc)
            return json.dumps({"ok": False, "error": f"建立群組失敗：{error}", "code": code}, ensure_ascii=False)
        self._upsert_group(group)
        self._emit("group_create", {"group": self._ser_group(group)})
        return json.dumps({"ok": True, "group": self._ser_group(group)}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def delete_group(self, group_id: str) -> str:
        return self._request_async("group_deleted", self._async_delete_group(group_id), {"group_id": group_id})

    async def _async_delete_group(self, group_id: str) -> str:
        try:
            await self.client.delete_group(group_id)
        except Exception as exc:
            logger.exception("[bridge] delete group failed for group %s", group_id)
            code = getattr(exc, "code", "") or ""
            error = getattr(exc, "message", "") or str(exc)
            return json.dumps({"ok": False, "error": f"刪除群組失敗：{error}", "code": code}, ensure_ascii=False)
        self._groups.pop(group_id, None)
        self._messages.pop(group_id, None)
        self._emit("group_delete", {"group_id": group_id})
        return json.dumps({"ok": True}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def join_group(self, group_id: str) -> str:
        return self._request_async("group_joined", self._async_join_group(group_id), {"group_id": group_id})

    async def _async_join_group(self, group_id: str) -> str:
        try:
            group = await self.client.join_group(group_id)
        except Exception as exc:
            logger.exception("[bridge] join group failed for group %s", group_id)
            code = getattr(exc, "code", "") or ""
            error = getattr(exc, "message", "") or str(exc)
            return json.dumps({"ok": False, "error": f"加入群組失敗：{error}", "code": code}, ensure_ascii=False)
        self._upsert_group(group)
        self._emit("group_join", {
            "group_id": group.id,
            "member": {"id": self._current_user.get("id", ""), "name": self._current_user.get("name", "")},
        })
        return json.dumps({"ok": True, "group": self._ser_group(group)}, ensure_ascii=False)

    @pyqtSlot(str, result=str)
    def leave_group(self, group_id: str) -> str:
        return self._request_async("group_left", self._async_leave_group(group_id), {"group_id": group_id})

    async def _async_leave_group(self, group_id: str) -> str:
        try:
            await self.client.leave_group(group_id)
        except Exception as exc:
            logger.exception("[bridge] leave group failed for group %s", group_id)
            code = getattr(exc, "code", "") or ""
            error = getattr(exc, "message", "") or str(exc)
            return json.dumps({"ok": False, "error": f"退出群組失敗：{error}", "code": code}, ensure_ascii=False)
        self._groups.pop(group_id, None)
        self._messages.pop(group_id, None)
        self._emit("group_leave", {
            "group_id": group_id,
            "member": {"id": self._current_user.get("id", ""), "name": self._current_user.get("name", "")},
        })
        return json.dumps({"ok": True}, ensure_ascii=False)

