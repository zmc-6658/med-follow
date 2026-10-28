#!/usr/bin/env python3
"""MCP 通信集成测试：python test_mcp_integration.py（用 venv python，需 pyyaml）。

验证 8 步：协议握手与工具清单 → 读工具 → 写工具落库 → 鉴权与白名单。
写库断言通过 psql 直查数据库确认；测试产生的 demo 数据保留。
"""
import asyncio
import json
import os
import subprocess
import sys
from pathlib import Path

BASE = Path(__file__).resolve().parent            # ai-service/
sys.path.insert(0, str(BASE))

from core.mcp_client import MCPClient, MCPError, _default_settings  # noqa: E402

ROOT = BASE.parent                                 # MedFollow/
GATEWAY = ROOT / "gateway"
BIN = GATEWAY / "bin" / "mcpserver"
PSQL_ENV = dict(os.environ, PGPASSWORD="Zhong@123456")

passed = 0
failed = []


def check(name, cond, detail=""):
    global passed
    mark = "PASS" if cond else "FAIL"
    print(f"  [{mark}] {name}" + ("" if cond else f"  << {detail}"))
    if cond:
        passed += 1
    else:
        failed.append(name)


def psql(sql):
    out = subprocess.run(
        ["psql", "-h", "localhost", "-U", "zhong", "-d", "medfollow", "-t", "-A", "-c", sql],
        capture_output=True, text=True, env=PSQL_ENV)
    if out.returncode != 0:
        raise RuntimeError(f"psql failed: {out.stderr.strip()}")
    return out.stdout.strip()


def build_server():
    print("步骤0：编译 Go MCP Server")
    r = subprocess.run(["go", "build", "-o", str(BIN), "./cmd/mcpserver"],
                       cwd=GATEWAY, capture_output=True, text=True)
    check("go build ./cmd/mcpserver", r.returncode == 0, r.stderr.strip())
    return r.returncode == 0


async def main():
    if not build_server():
        return

    settings = _default_settings()

    # ---- 步骤1：initialize + tools/list ----
    print("\n步骤1：initialize + tools/list")
    client = await MCPClient(**settings).connect()
    tools = await client.list_tools()
    names = {t["name"] for t in tools}
    check("tools/list 返回恰好 6 个工具", len(tools) == 6, str(sorted(names)))
    check("工具名与 PRD 5.4 一致", names == {
        "query_patient", "list_tasks", "query_history_trend",
        "get_skill_config", "save_followup", "push_alert"}, str(sorted(names)))
    check("每个工具含入参 inputSchema",
          all(isinstance(t.get("inputSchema"), dict) and "properties" in t["inputSchema"]
              for t in tools))

    # ---- 步骤2：query_patient 不泄露证件号 ----
    print("\n步骤2：query_patient('HIS000001')")
    p = await client.call_tool("query_patient", {"patient_id": "HIS000001"})
    check("返回档案（name/patient_id）",
          bool(p) and p.get("name") and p.get("patient_id") == "HIS000001", json.dumps(p, ensure_ascii=False))
    check("不含 id_card 等证件号字段", "id_card" not in p)

    # ---- 步骤3：list_tasks ----
    print("\n步骤3：list_tasks(status='created')")
    tasks = await client.call_tool("list_tasks", {"status": "created"})
    check("返回任务列表", isinstance(tasks, list) and len(tasks) > 0,
          f"len={len(tasks) if isinstance(tasks, list) else tasks}")
    check("任务含 task_id/status 字段",
          all("task_id" in t and t.get("status") == "created" for t in tasks))

    # ---- 步骤4：query_history_trend 无记录患者返回空数组 ----
    print("\n步骤4：query_history_trend（无记录患者）")
    no_rec = psql(
        "SELECT patient_id FROM patients p "
        "WHERE NOT EXISTS (SELECT 1 FROM followup_records r WHERE r.patient_id = p.id) LIMIT 1")
    trend = await client.call_tool("query_history_trend", {"patient_id": no_rec})
    check(f"患者 {no_rec} 无记录返回空数组", trend == [], json.dumps(trend, ensure_ascii=False))

    # ---- 步骤5：get_skill_config ----
    print("\n步骤5：get_skill_config('hypertension')")
    skill = await client.call_tool("get_skill_config", {"disease_type": "hypertension"})
    check("返回四件套且 SKILL.md 非空",
          bool(skill.get("skill_md")) and bool(skill.get("schema_json"))
          and bool(skill.get("questions_yaml")) and bool(skill.get("rules_json")))

    # ---- 步骤6：save_followup 落 draft + 审计 ----
    print("\n步骤6：save_followup → 查库验证")
    task_id = psql(
        "SELECT task_id FROM followup_tasks ORDER BY (status = 'created') DESC, task_id LIMIT 1")
    audit_before = int(psql("SELECT count(*) FROM audit_logs WHERE user_id = 'ai-service'"))
    rec = await client.call_tool("save_followup", {
        "task_id": task_id,
        "structured_data": {"systolic": "135", "diastolic": "85"},
        "ai_raw": {"note": "integration-test"},
        "confidence": 0.92,
    })
    record_id = rec.get("record_id", "")
    row = psql(f"SELECT status || '/' || source FROM followup_records WHERE record_id = '{record_id}'")
    check(f"followup_records 新增行 status='draft'/source='ai'（task={task_id}）",
          row == "draft/ai", f"got '{row}'")
    audit_after = int(psql("SELECT count(*) FROM audit_logs WHERE user_id = 'ai-service'"))
    check("audit_logs 新增一条（caller=ai-service）", audit_after - audit_before == 1)

    # ---- 步骤7：push_alert 落 open ----
    print("\n步骤7：push_alert → 查库验证")
    alert = await client.call_tool("push_alert", {
        "task_id": task_id, "level": "yellow",
        "rule_id": "BP_HIGH_TEST", "reason": "集成测试告警"})
    alert_id = alert.get("alert_id", "")
    row = psql(f"SELECT status || '/' || level FROM alerts WHERE alert_id = '{alert_id}'")
    check(f"alerts 新增行 status='open'/level='yellow'（task={task_id}）",
          row == "open/yellow", f"got '{row}'")

    await client.close()

    # ---- 步骤8a：token 错误 → initialize 失败 ----
    print("\n步骤8：鉴权与白名单")
    bad = MCPClient(settings["server_binary"], settings["gateway_dir"], token="wrong-token")
    try:
        await bad.connect()
        check("token 错误时 initialize 失败", False, "connected unexpectedly")
    except MCPError as e:
        check("token 错误时 initialize 失败", e.code == -32001, f"code={e.code}")

    # ---- 步骤8b：白名单删掉 save_followup → -32601（测完恢复白名单）----
    orig = (GATEWAY / "etc" / "config.yaml").read_text(encoding="utf-8")
    assert orig.count("save_followup, ") == 1, "config.yaml 白名单格式变化，测试需同步调整"
    tmp = Path("/tmp/mcp_test_config.yaml")
    tmp.write_text(orig.replace("save_followup, ", ""), encoding="utf-8")
    restricted = MCPClient(settings["server_binary"], settings["gateway_dir"],
                           token=settings["token"], config_path=tmp)
    try:
        await restricted.connect()
        await restricted.call_tool("save_followup", {"task_id": task_id, "structured_data": {}})
        check("白名单外工具返回 -32601", False, "call succeeded unexpectedly")
    except MCPError as e:
        check("白名单外工具返回 -32601 permission denied", e.code == -32601, f"code={e.code}")
    finally:
        await restricted.close()
        tmp.unlink(missing_ok=True)   # 临时配置只指向 /tmp，正式白名单未被改动

    # ---- 汇总 ----
    print(f"\n===== 通过 {passed} 项断言，失败 {len(failed)} 项 =====")
    if failed:
        print("失败项：" + "; ".join(failed))
        sys.exit(1)


if __name__ == "__main__":
    asyncio.run(main())
