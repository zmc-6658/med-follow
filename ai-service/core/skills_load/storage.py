import json
from typing import Optional, Dict, List
import redis.asyncio as redis
from .config import config
from models.skills_loader import  SkillFullContent, SkillMeta


class RedisStorage:
    def __init__(self):
        self.rd = redis.from_url(config.REDIS_URL)
        self.prefix = config.REDIS_PREFIX

    def _skill_cache_key(self, skill_name: str) -> str:
        return f"{self.prefix}content:{skill_name}"

    async def save_meta_index(self, meta_list: List[SkillMeta]):
        """将全量Lv1元索引存入Redis"""
        raw = [
            {
                "name": m.name,
                "disease": m.disease,
                "version": m.version,
                "description": m.description,
                "trigger_keywords": m.trigger_keywords,
                "file_path": m.file_path,
                "mtime": m.mtime,
                "references": m.references
            }
            for m in meta_list
        ]
        await self.rd.set(config.REDIS_INDEX_KEY, json.dumps(raw, ensure_ascii=False))

    async def load_meta_index(self) -> List[SkillMeta]:
        """读取Redis中的元索引"""
        data = await self.rd.get(config.REDIS_INDEX_KEY)
        if not data:
            return []
        arr = json.loads(data)
        return [SkillMeta(**item) for item in arr]

    async def save_full_skill(self, skill: SkillFullContent):
        """保存Lv2完整skill到Redis"""
        raw = {
            "meta": {
                "name": skill.meta.name,
                "disease": skill.meta.disease,
                "version": skill.meta.version,
                "description": skill.meta.description,
                "trigger_keywords": skill.meta.trigger_keywords,
                "file_path": skill.meta.file_path,
                "mtime": skill.meta.mtime,
                "references": skill.meta.references
            },
            "raw_md": skill.raw_md,
            "refs": skill.refs
        }
        await self.rd.set(self._skill_cache_key(skill.meta.name), json.dumps(raw, ensure_ascii=False))

    async def load_full_skill(self, skill_name: str) -> Optional[SkillFullContent]:
        data = await self.rd.get(self._skill_cache_key(skill_name))
        if not data:
            return None
        d = json.loads(data)
        meta = SkillMeta(**d["meta"])
        return SkillFullContent(meta=meta, raw_md=d["raw_md"], refs=d["refs"])

    async def invalidate_skill(self, skill_name: str):
        await self.rd.delete(self._skill_cache_key(skill_name))

    async def close(self):
        await self.rd.aclose()
