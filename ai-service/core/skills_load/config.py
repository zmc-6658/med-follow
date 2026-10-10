import yaml
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parents[2]

_cfg = yaml.safe_load((BASE_DIR / "etc" / "config.yaml").read_text(encoding="utf-8"))

_skill_root = Path(_cfg["skill_root"]).expanduser()
if not _skill_root.is_absolute():
    _skill_root = BASE_DIR / _skill_root


class SkillLoadConfig:
    SKILL_ROOT = str(_skill_root.resolve())
    REDIS_URL = _cfg["redis_url"]
    REDIS_PREFIX = _cfg["redis_prefix"]
    REDIS_INDEX_KEY = f"{REDIS_PREFIX}meta_index"
    L1_MAX_LRU_SIZE = _cfg["l1_max_lru_size"]
    ENABLE_REAL_TIME_MTIME_CHECK = _cfg["enable_real_time_mtime_check"]
    MTIME_CHECK_COOLDOWN_SEC = _cfg["mtime_check_cooldown_sec"]
    META_INDEX_REFRESH_INTERVAL = _cfg["meta_index_refresh_interval"]


config = SkillLoadConfig()
