
class _CacheBase:
    def __init__(self):
        self.token: str = ""
        self.data: dict = {}

    def has_token(self) -> bool:
        return bool(self.token)

    def has_data(self) -> bool:
        return bool(self.data)
        
_cache_base = _CacheBase()