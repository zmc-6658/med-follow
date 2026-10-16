-- 0002_table_comments.sql
-- 全部业务表及字段的中文备注（COMMENT ON 语句天然幂等，可重复执行）

-- ============ patients 患者档案 ============
COMMENT ON TABLE patients IS '患者档案表（高血压 MVP）';
COMMENT ON COLUMN patients.id IS '主键（uuid）';
COMMENT ON COLUMN patients.patient_id IS '业务患者编号（本系统对外标识，Mock 下等于 his_patient_id，如 HIS000001）';
COMMENT ON COLUMN patients.name IS '患者姓名';
COMMENT ON COLUMN patients.name_display IS '脱敏显示名（如 张*明，用于前端展示）';
COMMENT ON COLUMN patients.gender IS '性别（male/female）';
COMMENT ON COLUMN patients.birth_date IS '出生日期';
COMMENT ON COLUMN patients.age IS '年龄（周岁，同步时按 birth_date 计算）';
COMMENT ON COLUMN patients.phone IS '患者本人手机号（短信触达主号码）';
COMMENT ON COLUMN patients.phone_alt IS '第二联系人手机号（家属等，不用于直接短信触达）';
COMMENT ON COLUMN patients.disease_type IS '管理病种（本阶段仅 hypertension 高血压）';
COMMENT ON COLUMN patients.diagnosis_date IS '确诊日期';
COMMENT ON COLUMN patients.course_years IS '病程年限（年）';
COMMENT ON COLUMN patients.allergy_history IS '过敏史';
COMMENT ON COLUMN patients.complication_history IS '并发症史';
COMMENT ON COLUMN patients.current_medications IS '当前用药（文本，"药名 剂量 频次"分号分隔，同步时由 HIS 用药渲染）';
COMMENT ON COLUMN patients.managing_doctor IS '管理医生姓名';
COMMENT ON COLUMN patients.health_center IS '所属社区卫生服务中心';
COMMENT ON COLUMN patients.created_at IS '创建时间';
COMMENT ON COLUMN patients.updated_at IS '更新时间';
COMMENT ON COLUMN patients.his_patient_id IS 'HIS 患者唯一标识（同步 upsert 幂等键，唯一约束）';
COMMENT ON COLUMN patients.phone_source IS '电话来源（his=以 HIS 为准 / manual=人工修正，manual 的电话不被同步覆盖）';

-- ============ followup_tasks 随访任务 ============
COMMENT ON TABLE followup_tasks IS '随访任务表（按病种+季度生成）';
COMMENT ON COLUMN followup_tasks.id IS '主键（uuid）';
COMMENT ON COLUMN followup_tasks.task_id IS '业务任务编号（格式 FT-{patient_id}-{period_key}，如 FT-HIS000001-2026Q4）';
COMMENT ON COLUMN followup_tasks.patient_id IS '患者 id（外键 patients.id）';
COMMENT ON COLUMN followup_tasks.disease_type IS '随访病种（hypertension）';
COMMENT ON COLUMN followup_tasks.followup_round IS '随访轮次（当年同病种第几次，生成时=当年已有任务数+1）';
COMMENT ON COLUMN followup_tasks.plan_date IS '计划随访日期（生成时取下一个工作日；驱动 created→scheduled 推进与逾期判断）';
COMMENT ON COLUMN followup_tasks.actual_date IS '实际随访日期（患者提交/医生完成时回填）';
COMMENT ON COLUMN followup_tasks.channel IS '触达渠道（sms=患者本人手机号合法 / manual=无有效手机号走人工随访）';
COMMENT ON COLUMN followup_tasks.status IS '任务状态（created→scheduled→sent→opened→completed，失败/失访走 lost，见 PRD 附录C）';
COMMENT ON COLUMN followup_tasks.token IS '一次性随访链接 Token（患者点短信链接时校验，未生成前为空）';
COMMENT ON COLUMN followup_tasks.token_expired IS 'Token 是否已过期失效（true 后链接不可再访问）';
COMMENT ON COLUMN followup_tasks.lost_reason IS '失访原因（status=lost 时必填）';
COMMENT ON COLUMN followup_tasks.created_at IS '创建时间';
COMMENT ON COLUMN followup_tasks.updated_at IS '更新时间';
COMMENT ON COLUMN followup_tasks.sent_at IS '短信发送时间';
COMMENT ON COLUMN followup_tasks.opened_at IS '患者打开链接时间（首次点击）';
COMMENT ON COLUMN followup_tasks.completed_at IS '随访完成时间（提交通过校验时回填）';
COMMENT ON COLUMN followup_tasks.period_key IS '任务所属季度键（如 2026Q4；与 patient_id+disease_type 组成唯一约束防重）';

-- ============ followup_records 随访记录 ============
COMMENT ON TABLE followup_records IS '随访记录表（一次随访一份，AI 初稿+人工确认双版本）';
COMMENT ON COLUMN followup_records.id IS '主键（uuid）';
COMMENT ON COLUMN followup_records.record_id IS '业务记录编号';
COMMENT ON COLUMN followup_records.task_id IS '关联任务编号（外键 followup_tasks.task_id）';
COMMENT ON COLUMN followup_records.patient_id IS '患者 id（外键 patients.id）';
COMMENT ON COLUMN followup_records.disease_type IS '随访病种';
COMMENT ON COLUMN followup_records.followup_round IS '随访轮次（冗余自任务，报表用）';
COMMENT ON COLUMN followup_records.followup_date IS '随访日期';
COMMENT ON COLUMN followup_records.followup_method IS '随访方式（sms 链接自填 / phone 电话 / visit 面访等）';
COMMENT ON COLUMN followup_records.structured_data IS '结构化随访数据（AI 解析后的 JSON）';
COMMENT ON COLUMN followup_records.source IS '记录来源（ai_draft=AI 初稿 / human=人工录入）';
COMMENT ON COLUMN followup_records.proxy IS '是否代办（家属代填为 true）';
COMMENT ON COLUMN followup_records.ai_raw IS 'AI 原始输出（大模型返回原文留档）';
COMMENT ON COLUMN followup_records.human_final IS '人工确认后的最终数据（归档以本字段为准）';
COMMENT ON COLUMN followup_records.completeness_score IS '完整度评分（0-100，AI 评估）';
COMMENT ON COLUMN followup_records.confidence IS 'AI 置信度（0-1）';
COMMENT ON COLUMN followup_records.status IS '记录状态（draft→submitted→confirmed）';
COMMENT ON COLUMN followup_records.followup_class IS '随访分类（控制满意/不满意/不良反应等）';
COMMENT ON COLUMN followup_records.risk_level IS '风险评估等级（low/mid/high，触发预警用）';
COMMENT ON COLUMN followup_records.alert_ids IS '触发产生的预警 id 列表';
COMMENT ON COLUMN followup_records.created_at IS '创建时间';
COMMENT ON COLUMN followup_records.submitted_at IS '患者提交时间';
COMMENT ON COLUMN followup_records.confirmed_at IS '医生确认时间';
COMMENT ON COLUMN followup_records.synced_at IS '回写 HIS 时间（P1 回写用，MVP 为空）';
COMMENT ON COLUMN followup_records.updated_at IS '更新时间';

-- ============ alerts 预警 ============
COMMENT ON TABLE alerts IS '预警表（规则引擎按随访数据触发）';
COMMENT ON COLUMN alerts.id IS '主键（uuid）';
COMMENT ON COLUMN alerts.alert_id IS '业务预警编号';
COMMENT ON COLUMN alerts.record_id IS '触发预警的随访记录编号';
COMMENT ON COLUMN alerts.task_id IS '关联任务编号';
COMMENT ON COLUMN alerts.patient_id IS '患者 id（外键 patients.id）';
COMMENT ON COLUMN alerts.disease_type IS '病种';
COMMENT ON COLUMN alerts.level IS '预警级别（low/mid/high，影响处置时效要求）';
COMMENT ON COLUMN alerts.rule_id IS '命中的规则编号';
COMMENT ON COLUMN alerts.rule_name IS '规则名称（如 收缩压≥180）';
COMMENT ON COLUMN alerts.trigger_fields IS '触发字段快照（jsonb，记录命中时的数值）';
COMMENT ON COLUMN alerts.suggestion IS '处置建议';
COMMENT ON COLUMN alerts.suggestion_src IS '建议来源（ai=AI 生成 / rule=规则内置）';
COMMENT ON COLUMN alerts.status IS '预警状态（open→assigned→handled，或 escalated）';
COMMENT ON COLUMN alerts.notified_at IS '通知医生时间';
COMMENT ON COLUMN alerts.assigned_to IS '指派给谁（医生）';
COMMENT ON COLUMN alerts.handled_by IS '处理人';
COMMENT ON COLUMN alerts.handled_at IS '处理时间';
COMMENT ON COLUMN alerts.handle_action IS '处置动作（confirmed 确认随访 / adjusted 调整方案 / referral 转诊等）';
COMMENT ON COLUMN alerts.handle_note IS '处理备注';
COMMENT ON COLUMN alerts.escalated_to IS '升级上报对象（上级医院/公卫科）';
COMMENT ON COLUMN alerts.created_at IS '创建时间';
COMMENT ON COLUMN alerts.updated_at IS '更新时间';

-- ============ diff_logs 差异日志 ============
COMMENT ON TABLE diff_logs IS 'AI 数据与人工确认的差异日志（对账与 AI 质量评估）';
COMMENT ON COLUMN diff_logs.id IS '主键（uuid）';
COMMENT ON COLUMN diff_logs.record_id IS '关联随访记录编号';
COMMENT ON COLUMN diff_logs.field_key IS '差异字段名';
COMMENT ON COLUMN diff_logs.ai_value IS 'AI 给出的值（jsonb）';
COMMENT ON COLUMN diff_logs.human_value IS '人工最终确认的值（jsonb）';
COMMENT ON COLUMN diff_logs.operator IS '操作人';
COMMENT ON COLUMN diff_logs.operation IS '操作类型（confirm 采纳 AI / override 人工改写）';
COMMENT ON COLUMN diff_logs.created_at IS '创建时间';

-- ============ audit_logs 审计日志 ============
COMMENT ON TABLE audit_logs IS '操作审计日志（等保合规要求）';
COMMENT ON COLUMN audit_logs.id IS '主键（uuid）';
COMMENT ON COLUMN audit_logs.user_id IS '操作用户标识';
COMMENT ON COLUMN audit_logs.user_role IS '操作用户角色';
COMMENT ON COLUMN audit_logs.action IS '操作类型（login/create/confirm/export 等）';
COMMENT ON COLUMN audit_logs.resource_type IS '操作资源类型';
COMMENT ON COLUMN audit_logs.resource_id IS '操作资源 id';
COMMENT ON COLUMN audit_logs.ip_address IS '来源 IP';
COMMENT ON COLUMN audit_logs.user_agent IS '客户端 UA';
COMMENT ON COLUMN audit_logs.request_body IS '请求体快照（jsonb，注意脱敏）';
COMMENT ON COLUMN audit_logs.response_body IS '响应体快照（jsonb，注意脱敏）';
COMMENT ON COLUMN audit_logs.created_at IS '创建时间';

-- ============ sms_logs 短信日志 ============
COMMENT ON TABLE sms_logs IS '短信触达日志（MVP 模拟发送）';
COMMENT ON COLUMN sms_logs.id IS '主键（uuid）';
COMMENT ON COLUMN sms_logs.task_id IS '关联任务编号';
COMMENT ON COLUMN sms_logs.patient_id IS '患者 id（外键 patients.id）';
COMMENT ON COLUMN sms_logs.phone IS '实际发送号码';
COMMENT ON COLUMN sms_logs.template IS '短信模板编号/内容标识';
COMMENT ON COLUMN sms_logs.short_link IS '短信短链（指向一次性 Token 随访页）';
COMMENT ON COLUMN sms_logs.sent_at IS '发送时间';
COMMENT ON COLUMN sms_logs.simulated_recv IS '是否模拟接收成功（MVP 不接真实短信网关）';
COMMENT ON COLUMN sms_logs.clicked IS '患者是否点击链接';
COMMENT ON COLUMN sms_logs.clicked_at IS '点击时间';
COMMENT ON COLUMN sms_logs.status IS '发送状态（sent/failed）';

-- ============ sync_state 同步水位 ============
COMMENT ON TABLE sync_state IS '数据同步水位表（单行，id=1）';
COMMENT ON COLUMN sync_state.id IS '固定为 1（单行表）';
COMMENT ON COLUMN sync_state.last_sync_at IS '增量同步水位（只拉 update_time 晚于该时间的档案；水位只前进不后退）';
COMMENT ON COLUMN sync_state.updated_at IS '水位更新时间';

-- ============ sync_log 同步批次日志 ============
COMMENT ON TABLE sync_log IS '数据同步批次日志（每轮同步一行）';
COMMENT ON COLUMN sync_log.id IS '主键（自增）';
COMMENT ON COLUMN sync_log.batch_id IS '批次号（时间戳+随机串，如 20261015105209-2b9135）';
COMMENT ON COLUMN sync_log.started_at IS '批次开始时间';
COMMENT ON COLUMN sync_log.finished_at IS '批次结束时间（进行中为空）';
COMMENT ON COLUMN sync_log.fetched IS '本批从数据源拉取条数';
COMMENT ON COLUMN sync_log.inserted IS '新增患者数';
COMMENT ON COLUMN sync_log.updated IS '更新患者数';
COMMENT ON COLUMN sync_log.skipped IS '跳过条数（缺必填字段/非高血压/单条失败）';
COMMENT ON COLUMN sync_log.error_summary IS '错误摘要（分号分隔，如 HIS000103: invalid phone format）';
