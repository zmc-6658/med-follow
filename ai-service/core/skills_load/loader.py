import os
import frontmatter
from pathlib import Path
from typing import List
from models.skills_loader import SkillMeta
from .exceptions import SkillFileReadError, SkillParseError
from .config import config


class DiskLoader:
    """磁盘文件加载器：扫描目录、解析SKILL.md frontmatter，读取md正文、读取附属参考文件"""
    def __init__(self):
        self.root = Path(config.SKILL_ROOT)

    async def scan_all_skill_meta(self) -> List[SkillMeta]:
        """
        扫描skills目录，只读取frontmatter，不加载完整md正文，生成Lv1元信息
        递归查找所有层级的 SKILL.md，约定每个SKILL.md所在目录即为skill目录
        """
        meta_list: List[SkillMeta] = []
        if not self.root.exists():
            return meta_list
        for md_path in self.root.rglob("SKILL.md"):
            skill_dir = md_path.parent
            try:
                stat = os.stat(md_path)
                mtime = stat.st_mtime
                fm = frontmatter.load(md_path)
                meta = SkillMeta(
                    name=fm["name"],
                    disease=fm["disease"],
                    version=str(fm["version"]),
                    description=fm["description"],
                    trigger_keywords=fm["trigger_keywords"],
                    file_path=str(md_path),
                    mtime=mtime,
                    references=fm.get("references", [])
                )
                meta_list.append(meta)
            except Exception as e:
                raise SkillParseError(skill_name=skill_dir.name, err=str(e))
        return meta_list

    async def read_skill_md(self, meta: SkillMeta) -> str:
        """读取SKILL.md完整正文"""
        p = Path(meta.file_path)
        try:
            fm = frontmatter.load(p)
            return fm.content
        except Exception as e:
            raise SkillFileReadError(str(p), str(e))

    async def read_reference_file(self, meta: SkillMeta, ref_relative: str) -> str:
        """
        读取skill附属参考文件
        meta: Lv1元信息，附属文件相对于其SKILL.md所在目录解析
        ref_relative: 相对skill目录的路径，如 schema.json , questions.yaml , kb/guide.md
        """
        skill_dir = Path(meta.file_path).parent
        target = (skill_dir / ref_relative).resolve()
        # 防目录穿越：附属文件必须位于skill目录内
        if not target.is_relative_to(skill_dir.resolve()):
            from .exceptions import RefFileNotFoundError
            raise RefFileNotFoundError(meta.name, ref_relative)
        if not target.exists() or not target.is_file():
            from .exceptions import RefFileNotFoundError
            raise RefFileNotFoundError(meta.name, ref_relative)
        try:
            return target.read_text(encoding="utf-8")
        except Exception as e:
            raise SkillFileReadError(str(target), str(e))

    async def get_file_mtime(self, file_path: str) -> float:
        return os.stat(file_path).st_mtime
