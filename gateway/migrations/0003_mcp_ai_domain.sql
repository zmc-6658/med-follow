-- 0003_mcp_ai_domain.sql
-- MCP AI 域增量：扩展两处 CHECK 约束取值，供 MCP 写工具（save_followup / push_alert）使用。
-- 说明：不改任何既有列与数据，仅放宽约束取值；可重复执行（先 DROP 后 ADD 天然幂等）。

-- followup_records.source 增加 'ai'：AI 经 MCP 写入的记录标记来源为 ai，
-- 且仅允许落 draft 域（状态推进到 submitted/confirmed 仍须人工确认，MCP 无此工具）
ALTER TABLE followup_records DROP CONSTRAINT IF EXISTS followup_records_source_check;
ALTER TABLE followup_records ADD CONSTRAINT followup_records_source_check
  CHECK (source::text = ANY (ARRAY['self_reported'::text, 'proxy_family'::text, 'proxy_doctor'::text,
                                   'proxy_nurse'::text, 'ai'::text]));

-- alerts.status 增加 'open'：MCP push_alert 写入的初始告警状态，等待人工处理流转
ALTER TABLE alerts DROP CONSTRAINT IF EXISTS alerts_status_check;
ALTER TABLE alerts ADD CONSTRAINT alerts_status_check
  CHECK (status::text = ANY (ARRAY['created'::text, 'notified'::text, 'assigned'::text, 'handling'::text,
                                   'resolved'::text, 'ignored'::text, 'escalated'::text, 'open'::text]));

-- alerts.record_id 放开为可空：NULL 表示对话实时预警、随访记录尚未生成（红级主路径）
ALTER TABLE alerts ALTER COLUMN record_id DROP NOT NULL;
