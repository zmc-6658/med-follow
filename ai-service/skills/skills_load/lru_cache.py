from collections import OrderedDict
from typing import Generic, TypeVar, Optional
import asyncio

T = TypeVar("T")


class AsyncLRUCache(Generic[T]):
    """
    L1 进程本地LRU缓存
    增加async锁，解决多协程并发竞态问题
    """
    def __init__(self, max_size: int):
        self.max_size = max_size
        self._cache: OrderedDict[str, T] = OrderedDict()
        self._lock = asyncio.Lock()  # 协程锁

    async def get(self, key: str) -> Optional[T]:
        async with self._lock:
            if key not in self._cache:
                return None
            # 命中，移动到末尾标记最近使用
            val = self._cache.pop(key)
            self._cache[key] = val
            return val

    async def put(self, key: str, value: T) -> Optional[str]:
        """写入缓存，如果触发淘汰，返回被淘汰的key，否则None"""
        async with self._lock:
            if key in self._cache:
                self._cache.pop(key)
            evicted_key: Optional[str] = None
            if len(self._cache) >= self.max_size:
                evicted_key, _ = self._cache.popitem(last=False)     # last=False表示FIFO，弹出最旧的item 先进先出
            self._cache[key] = value
            return evicted_key

    async def remove(self, key: str):  
        async with self._lock:
            if key in self._cache:
                del self._cache[key]

    async def clear(self):
        async with self._lock:
            self._cache.clear()

    @property
    def size(self) -> int:
        # 读取size也要上锁，防止并发修改
        async def _get_size():
            async with self._lock:
                return len(self._cache)
        return asyncio.run(_get_size())
