from pydantic_settings import BaseSettings
import os


class SkillLoadConfig(BaseSettings):
    # skill根目录
    SKILL_ROOT: str = os.path.expanduser("~/桌面/MedFollow/ai-service/skills")
    # Redis连接
    REDIS_URL: str = "redis://127.0.0.1:6379/0"
    REDIS_PREFIX: str = "medfollow:skill:"

    # L1本地LRU缓存最大容量，默认5
    L1_MAX_LRU_SIZE: int = 5

    # ===== mtime校验配置【修复问题2】=====
    ENABLE_REAL_TIME_MTIME_CHECK: bool = False  # 生产默认关闭实时stat
    MTIME_CHECK_COOLDOWN_SEC: int = 60  # 冷却窗口，60s内不再重复stat同一个文件

    # ===== 元索引自动刷新【修复问题4】=====
    META_INDEX_REFRESH_INTERVAL: int = 120  # 后台定时任务，每120s从Redis刷新本地meta索引

    # Redis Key定义
    REDIS_INDEX_KEY: str = f"{REDIS_PREFIX}meta_index"

    class Config:
        env_prefix = "SKILL_LOAD_"


config = SkillLoadConfig()
