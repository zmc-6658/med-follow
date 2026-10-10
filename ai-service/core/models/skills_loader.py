from dataclasses import dataclass, field
from typing import List, Dict, Any, Optional
import time


@dataclass
class SkillMeta:
    """Lv1 仅元数据，扫描索引阶段加载，不包含正文/附件"""
    name: str
    disease: str
    version: str
    description: str
    trigger_keywords: List[str]
    file_path: str
    mtime: float  # 文件修改时间戳
    references: List[Dict[str, str]] = field(default_factory=list)  # frontmatter声明的附属文件清单 [{file, purpose}]
    last_check_ts: float = field(default_factory=time.time) # 新增：上次磁盘stat校验时间


@dataclass
class SkillFullContent:
    """Lv2：meta + md原文；Lv3在此基础上可以追加refs"""
    meta: SkillMeta
    raw_md: str
    # refs 不会默认加载；调用load_skill_reference才填充
    refs: Dict[str, str] = field(default_factory=dict)


@dataclass
class CacheMetric:
    """埋点指标，观测命中率、淘汰次数"""
    l1_hit: int = 0
    l1_miss: int = 0
    evict_count: int = 0
    redis_hit: int = 0
    redis_miss: int = 0

    def to_dict(self) -> Dict[str, Any]:
        return self.__dict__
