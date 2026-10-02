_allowed_status = [200, 201, 202, 204]

def is_not_allowed_code(code: int) -> bool:
    return bool(code not in _allowed_status)

def raise_not_allowed_error(code: int):
    raise