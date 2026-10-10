class SkillLoadError(Exception):
    def __init__(self, code: str, msg: str):
        self.code = code
        self.msg = msg
        super().__init__(f"[{code}] {msg}")

    def to_llm_dict(self) -> dict:
        return {
            "error_code": self.code,
            "message": self.msg
        }


class SkillNotFoundError(SkillLoadError):
    def __init__(self, skill_name: str):
        super().__init__("SKILL_NOT_FOUND", f"skill {skill_name} not found in index")


class SkillFileReadError(SkillLoadError):
    def __init__(self, path: str, err: str):
        super().__init__("FILE_READ_FAIL", f"read skill file failed path={path}, err={err}")


class SkillParseError(SkillLoadError):
    def __init__(self, skill_name: str, err: str):
        super().__init__("PARSE_FAIL", f"parse skill frontmatter failed skill={skill_name}, err={err}")


class RefFileNotFoundError(SkillLoadError):
    def __init__(self, skill_name: str, ref_path: str):
        super().__init__("REF_NOT_FOUND", f"reference file not found skill={skill_name}, ref={ref_path}")
