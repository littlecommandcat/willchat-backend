class WillClientException(Exception):
    pass

class UnExpectedBody(WillClientException):
    pass

class FailedConnection(WillClientException):
    pass