import asyncio
from .skills_load import SkillsLoad
from .exceptions import SkillLoadError


async def main():
    sl = SkillsLoad()
    # 启动后台索引刷新任务
    await sl.start_background_index_refresh()

    # 1.启动扫描索引（Lv1元索引写入Redis）
    await sl.scan_index()
    print("✅ scan index done")

    # 2.关键词匹配召回（Lv1，只返回meta）
    candidates = await sl.match_by_trigger("高血压随访")
    print(f"匹配候选：{[c.name for c in candidates]}")

    # 3.Lv1 只拿元数据
    meta = await sl.get_skill("hypertension-followup-skill", disclose_level=1)
    print(f"\nLv1 meta name={meta.name}, version={meta.version}")

    # 4.Lv2 获取完整md正文
    skill_full = await sl.get_skill("hypertension-followup-skill", disclose_level=2)
    print(f"\nLv2 md长度：{len(skill_full.raw_md)}")

    # 5.Lv3 按需加载附属文件示例
    ref_text = await sl.load_skill_reference("hypertension-followup-skill", "schema.json")
    print("附属文件内容", ref_text)

    # 6.手动失效缓存
    await sl.invalidate_skill("hypertension-followup-skill")

    # 指标查看
    print("\n📊 metrics: ", sl.metric.to_dict())

    await sl.close()


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except SkillLoadError as e:
        print("Skill load error -> LLM dict: ", e.to_llm_dict())
