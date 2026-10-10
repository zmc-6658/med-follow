from typing import List, Optional
import asyncio, time
from .lru_cache import AsyncLRUCache
from .storage import RedisStorage
from .loader import DiskLoader
from models.skills_loader import  SkillMeta, SkillFullContent, CacheMetric
from .exceptions import SkillNotFoundError
from .config import config


class SkillsLoad:
    def __init__(self):
        self.l1_cache = AsyncLRUCache[SkillFullContent](max_size=config.L1_MAX_LRU_SIZE)
        self.redis_store = RedisStorage()
        self.disk_loader = DiskLoader()
        self.metric = CacheMetric()
        self._meta_index: List[SkillMeta] = []
        self._index_refresh_task: Optional[asyncio.Task] = None

    async def start_background_index_refresh(self):
        """启动后台定时任务，自动从Redis刷新本地meta索引【修复问题4】"""
        async def _refresh_loop():
            while True:
                try:
                    await self.reload_index_from_redis()
                except Exception:
                    pass
                await asyncio.sleep(config.META_INDEX_REFRESH_INTERVAL)
        self._index_refresh_task = asyncio.create_task(_refresh_loop())

    async def stop_background_task(self):
        if self._index_refresh_task:
            self._index_refresh_task.cancel()
            try:
                await self._index_refresh_task
            except asyncio.CancelledError:
                pass

    async def scan_index(self):
        """
        预扫描全量skill，只提取Lv1元数据，写入Redis索引
        不加载md正文和附件；服务启动调用一次
        """
        self._meta_index = await self.disk_loader.scan_all_skill_meta()
        await self.redis_store.save_meta_index(self._meta_index)

    async def reload_index_from_redis(self):
        """从Redis重新加载元索引（多实例同步）"""
        self._meta_index = await self.redis_store.load_meta_index()

    async def match_by_trigger(self, text: str) -> List[SkillMeta]:
        """
        Lv1召回：根据用户输入文本，匹配trigger_keywords，返回候选skill元信息
        仅使用meta索引，不加载正文
        """
        candidates: List[SkillMeta] = []
        lower_text = text.lower()
        for meta in self._meta_index:
            for kw in meta.trigger_keywords:
                if kw.lower() in lower_text:
                    candidates.append(meta)
                    break
        return candidates

    async def get_skill(self, skill_name: str, disclose_level: int = 1) -> SkillMeta | SkillFullContent:
        """
        渐进式披露
        disclose_level=1 → Lv1 仅SkillMeta（召回阶段）
        disclose_level=2 → Lv2 meta + 完整md原文
        disclose_level=3 → Lv2基础对象，refs是空字典；refs通过load_skill_reference单独加载
        """
        # Lv1 直接返回meta，不走正文缓存
        if disclose_level == 1:
            for m in self._meta_index:
                if m.name == skill_name:
                    return m
            raise SkillNotFoundError(skill_name)

        # >=Lv2 需要完整SkillFullContent，走L1->L2->L3缓存链路
        skill_meta: Optional[SkillMeta] = None
        for m in self._meta_index:
            if m.name == skill_name:
                skill_meta = m
                break
        if skill_meta is None:
            raise SkillNotFoundError(skill_name)

        # L1 本地LRU缓存
        skill_full = await self.l1_cache.get(skill_name)
        if skill_full is not None:
            self.metric.l1_hit += 1
            # ===== mtime冷却校验逻辑【修复问题2】=====
            need_check = False
            if config.ENABLE_REAL_TIME_MTIME_CHECK:
                need_check = True
            else:
                now = time.time()
                if now - skill_full.meta.last_check_ts > config.MTIME_CHECK_COOLDOWN_SEC:
                    need_check = True
                    skill_full.meta.last_check_ts = now

            if need_check:
                disk_mtime = await self.disk_loader.get_file_mtime(skill_meta.file_path)
                if disk_mtime > skill_full.meta.mtime:
                    await self.invalidate_skill(skill_name)
                else:
                    return skill_full
            else:
                return skill_full
        self.metric.l1_miss += 1

        # L2 Redis缓存
        skill_full = await self.redis_store.load_full_skill(skill_name)
        if skill_full is not None:
            self.metric.redis_hit += 1
            need_check = False
            if config.ENABLE_REAL_TIME_MTIME_CHECK:
                need_check = True
            else:
                now = time.time()
                if now - skill_full.meta.last_check_ts > config.MTIME_CHECK_COOLDOWN_SEC:
                    need_check = True
                    skill_full.meta.last_check_ts = now

            if need_check:
                disk_mtime = await self.disk_loader.get_file_mtime(skill_meta.file_path)
                if disk_mtime > skill_full.meta.mtime:
                    await self.invalidate_skill(skill_name)
                else:
                    # 写入L1本地缓存
                    evicted_key = await self.l1_cache.put(skill_name, skill_full)
                    if evicted_key is not None:
                        self.metric.evict_count += 1
                    return skill_full
            else:
                evicted_key = await self.l1_cache.put(skill_name, skill_full)
                if evicted_key is not None:
                    self.metric.evict_count += 1
                self.metric.redis_hit +=1
                return skill_full
        self.metric.redis_miss += 1

        # L3 磁盘加载
        raw_md = await self.disk_loader.read_skill_md(skill_meta)
        skill_full = SkillFullContent(meta=skill_meta, raw_md=raw_md)
        # 写入Redis L2
        await self.redis_store.save_full_skill(skill_full)
        # 写入L1本地LRU
        evicted_key = await self.l1_cache.put(skill_name, skill_full)
        if evicted_key is not None:
            self.metric.evict_count += 1
        return skill_full

    async def load_skill_reference(self, skill_name: str, ref_relative_path: str) -> str:
        """Lv3按需加载单个附属参考文件（schema.json / questions.yaml / kb切片）
        附属文件须在SKILL.md frontmatter的references中声明
        """
        # 校验skill存在，并从meta获取SKILL.md所在目录
        meta = await self.get_skill(skill_name, disclose_level=1)
        # 校验是否在frontmatter声明的references清单内
        declared = {r.get("file") for r in meta.references}
        if declared and ref_relative_path not in declared:
            raise SkillNotFoundError(f"{skill_name} 未声明附属文件: {ref_relative_path}")
        return await self.disk_loader.read_reference_file(meta, ref_relative_path)

    async def invalidate_skill(self, skill_name: str):
        """手动清理L1本地缓存 + L2 Redis缓存，发布/运维调用"""
        await self.l1_cache.remove(skill_name)
        await self.redis_store.invalidate_skill(skill_name)

    async def close(self):
        await self.stop_background_task()
        await self.redis_store.close()
