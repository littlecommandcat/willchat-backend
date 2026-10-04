from typing import Any

_allowed_status = [200, 201, 202, 204]

def is_not_allowed_code(code: int) -> bool:
    return bool(code not in _allowed_status)

def raise_not_allowed_error(code: int):
    raise

class _Missing:
    __slots__ = ()

    def __eq__(self, other) -> bool:
        return False

    def __bool__(self) -> bool:
        return False

    def __hash__(self) -> int:
        return 0

    def __repr__(self):
        return '...'

MISSING: Any = _Missing()