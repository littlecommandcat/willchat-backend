class WillClientException(Exception):
    pass

class UnexpectedBody(WillClientException):
    pass

class UnexpectedStatus(WillClientException):
    pass

class FailedConnection(WillClientException):
    pass

# Unknown
class WillClientUnknown(WillClientException):
    pass

class UnknownGroup(WillClientUnknown):
    pass

class UnknownMessage(WillClientUnknown):
    pass

class UnknownMember(WillClientUnknown):
    pass

class UnknownLogin(WillClientUnknown):
    pass

class UnknownSession(WillClientUnknown):
    pass

class UnknownClient(WillClientUnknown):
    pass

# Forbidden
class WillClientForbidden(WillClientException):
    pass

class WillClientUnauthorized(WillClientException):
    pass

# Other
class TooManyRequests(WillClientException):
    pass


class APIError(WillClientException):
    """API error carrying the backend error code and message."""
    def __init__(self, code: str, message: str):
        super().__init__(message or code or "request failed")
        self.code = code
        self.message = message or code or "request failed"