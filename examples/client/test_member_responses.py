import ast
import importlib
import json
import pathlib
import sys
import types

root = pathlib.Path(__file__).resolve().parents[2]
desktop = root / "apps/desktop/rust"
assert json.loads((desktop / "package.json").read_text())["version"] == "1.1.3"
lock = json.loads((desktop / "package-lock.json").read_text())
assert lock["version"] == lock["packages"][""]["version"] == "1.1.3"
assert json.loads((desktop / "src-tauri/tauri.conf.json").read_text())["version"] == "1.1.3"
config = types.ModuleType("config")
config.BASE_URL = "http://localhost:8080"
config.EMAIL = ""
config.PASSWORD = ""
sys.modules["config"] = config
for relative in ("examples/client", "apps/desktop/python"):
    base = root / relative
    files = list(base.rglob("*.py"))
    for path in files:
        ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    sys.path.insert(0, str(base))
    package = importlib.import_module("willclient")
    models = importlib.import_module("willclient.models")
    alice = {"id": "mr-1", "name": "Alice", "email": "", "groups": [], "block": [], "read": {}}
    group = {"group_id": "gr-1", "name": "Chat", "owner": alice, "created_at": 1}
    member = models.Member.from_payload({"member": {**alice, "groups": [group], "block": [alice]}})
    assert member.groups[0].owner.name == "Alice"
    assert member.groups[0].owner_id == "mr-1"
    assert member.block[0].id == "mr-1"
    message = models.Message.from_payload({"author": alice, "reply_to": {"id": "ms-1", "author": alice, "content": "Hi", "type": 0}})
    assert message.author.name == "Alice" and message.authorid == "mr-1"
    assert message.reply_to.author.name == "Alice"
    legacy = models.Message.from_payload({"authorid": "mr-1"})
    assert legacy.authorid == "mr-1"
    assert package.__version__ == "1.1.3"
    print(relative, "syntax and member/group/message parsing OK", len(files), "files")
    sys.path.pop(0)
    for name in list(sys.modules):
        if name == "willclient" or name.startswith("willclient."):
            del sys.modules[name]