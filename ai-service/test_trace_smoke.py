#!/usr/bin/env python3
"""Trace 埋点 smoke 测试：venv/bin/python test_trace_smoke.py。

覆盖：JSON 输出合法性（每行可解析）、hashID 一致性（sha256 前 16 位、稳定）、
无 PII 明文（name/phone/id_card 键与值均不出现）、W3C 语义下父子 span 同 trace_id、
P1 预留 LLM 字段（prompt_tokens/completion_tokens/model/cost）可写入 span 属性。
"""
import io
import json
import re
import sys
from pathlib import Path

BASE = Path(__file__).resolve().parent
sys.path.insert(0, str(BASE))

from core import trace  # noqa: E402

passed = 0
failed = []


def check(name, cond, detail=""):
    global passed
    print(f"  [{'PASS' if cond else 'FAIL'}] {name}" + ("" if cond else f"  << {detail}"))
    if cond:
        passed += 1
    else:
        failed.append(name)


# ---- 1. hashID 一致性 ----
print("\n1：hashID 稳定性与格式")
h1, h2 = trace.hash_id("task-001"), trace.hash_id("task-001")
check("同输入两次哈希一致", h1 == h2)
check("16 位十六进制（sha256 前 16 位）", bool(re.fullmatch(r"[0-9a-f]{16}", h1)), h1)
check("与 sha256 标准实现一致",
      h1 == __import__("hashlib").sha256(b"task-001").hexdigest()[:16])
check("不同输入哈希不同", trace.hash_id("task-002") != h1)
check("哈希不含原 ID 明文", "task-001" not in h1)

# ---- 2. span 导出：父子链路 + PII 过滤 + JSON 合法性 ----
print("\n2：span 导出与 PII 过滤")
out = io.StringIO()
trace.init("medfollow-smoke-test", output=out)

# 标识按约定先 hash_id 再进 span（trace 中不出现明文 ID）
with trace.span("smoke.parent", patient_id=trace.hash_id("HIS000001")) as parent:
    parent_id = format(parent.get_span_context().span_id, "016x")
    with trace.span("smoke.child") as child:
        child_id = format(child.get_span_context().span_id, "016x")
        # PII 键与明文值均不应出现在输出中
        trace.emit_event(trace.EVENT_TURN_COMPLETED,
                         name="张三", phone="13800138000",
                         id_card="110101199001011234",
                         rounds=3, task_id=trace.hash_id("task-001"))
        # P1 预留 LLM 字段约定
        trace.emit_event("llm_call", prompt_tokens=120, completion_tokens=45,
                         model="test-model", cost=0.0021)

trace.shutdown()
raw = out.getvalue()

lines = [l for l in raw.splitlines() if l.strip()]
spans = []
ok_json = True
for l in lines:
    try:
        spans.append(json.loads(l))
    except json.JSONDecodeError:
        ok_json = False
check(f"导出 {len(lines)} 行均为合法 JSON", ok_json and len(spans) >= 2, raw[:200])

parent_row = next((s for s in spans if s["span_id"] == parent_id), None)
child_row = next((s for s in spans if s["span_id"] == child_id), None)
check("父子 span 均已导出", parent_row is not None and child_row is not None)
check("父子 span 同一 trace_id",
      parent_row and child_row and parent_row["trace_id"] == child_row["trace_id"])
check("child 的 parent_span_id 指向父 span",
      child_row and child_row["parent_span_id"] == parent_id)
check("trace_id 为 32 位十六进制（W3C）",
      bool(re.fullmatch(r"[0-9a-f]{32}", child_row["trace_id"])))

# ---- 3. 无 PII 明文 ----
print("\n3：PII 明文检查")
for secret in ("张三", "13800138000", "110101199001011234", "HIS000001", "task-001"):
    check(f"输出不含明文 {secret}", secret not in raw)

turn_event = next((e for s in spans for e in s.get("events", []) if e["name"] == "turn_completed"), None)
check("turn_completed 事件已记录", turn_event is not None)
check("事件中 PII 键被过滤",
      turn_event is not None
      and not ({"name", "phone", "id_card"} & set(turn_event["attributes"])),
      json.dumps(turn_event, ensure_ascii=False) if turn_event else "")
check("事件保留非 PII 属性（rounds/task_id）",
      turn_event is not None
      and turn_event["attributes"].get("rounds") == 3
      and len(turn_event["attributes"].get("task_id", "")) == 16)

# ---- 4. P1 预留 LLM 字段 ----
print("\n4：P1 预留字段约定")
llm_event = next((e for s in spans for e in s.get("events", [])
                  if e["name"] == "llm_call"), None)
check("llm 事件含 prompt_tokens/completion_tokens/model/cost",
      llm_event is not None
      and llm_event["attributes"].get("prompt_tokens") == 120
      and llm_event["attributes"].get("completion_tokens") == 45
      and llm_event["attributes"].get("model") == "test-model"
      and llm_event["attributes"].get("cost") == 0.0021,
      json.dumps(llm_event, ensure_ascii=False) if llm_event else "")

# ---- 汇总 ----
print(f"\n===== 通过 {passed} 项断言，失败 {len(failed)} 项 =====")
if failed:
    print("失败项：" + "; ".join(failed))
    sys.exit(1)
