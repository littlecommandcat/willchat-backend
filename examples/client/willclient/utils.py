from __future__ import annotations
from typing import Any
from .models.enums import ErrorType

from .exceptions import (
    WillClientException,
    WillClientForbidden,
    WillClientUnauthorized,
    WillClientUnknown,
    UnknownGroup,
    UnknownMessage,
    UnknownMember,
    UnknownLogin,
    TooManyRequests,
)

_allowed_status = [200, 201, 202, 204]


def _build_error_map():
    # No lazy import anymore 

    return {
        ErrorType.GROUP: {
            403: (WillClientForbidden, "Forbidden to fetch group"),
            404: (UnknownGroup, "Unknown group"),
        },
        ErrorType.MESSAGE: {
            403: (WillClientForbidden, "Forbidden to fetch message"),
            404: (UnknownMessage, "Unknown message"),
        },
        ErrorType.MEMBER: {
            403: (WillClientForbidden, "Forbidden to fetch member"),
            404: (UnknownMember, "Unknown member"),
        },
        ErrorType.AUTH: {
            403: (WillClientForbidden, "Forbidden to login"),
            404: (UnknownLogin, "Unknown login"),
        },
        ErrorType.OTHER: {
            401: (WillClientUnauthorized, "Unauthorized request"),
            403: (WillClientForbidden, "Forbidden to fetch data"),
            404: (WillClientUnknown, "Unknown error type for data"),
            405: (WillClientException, "Method not allowed while fetching data"),
            429: (TooManyRequests, "Request times out of credits"),
            500: (WillClientException, "WillClient internal error"),
        },
    }


_error_map = None


def _get_error_map():
    global _error_map
    if _error_map is None:
        _error_map = _build_error_map()
    return _error_map


def is_not_allowed_code(code: int) -> bool:
    return bool(code not in _allowed_status)


def raise_code_error(code: int, ertype: ErrorType) -> None:

    error_map = _get_error_map()
    entry = error_map.get(ertype, {}).get(code) or error_map[ErrorType.OTHER].get(code)

    if entry is None:
        error_cls, message = WillClientException, "WillClient internal error"
    else:
        error_cls, message = entry

    raise error_cls(message)

class _Missing:
    __slots__ = ()

    def __eq__(self, other) -> bool:
        return isinstance(other, _Missing)

    def __bool__(self) -> bool:
        return False

    def __hash__(self) -> int:
        return 0

    def __int__(self) -> int:
        return 0

    def __repr__(self):
        return '...'

MISSING: Any = _Missing()