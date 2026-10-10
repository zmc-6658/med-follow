-- MedFollow 高血压随访系统 - PostgreSQL 建表脚本 (V1.2 迭代0启动版)
-- 包含：患者表、随访任务表、随访记录表、预警记录表、差分日志表、短信日志表、审计日志表

-- ============================================
-- 1. 患者表
-- ============================================
CREATE TABLE patients (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    patient_id      VARCHAR(64) NOT NULL,          -- HIS主索引
    name            VARCHAR(32) NOT NULL,           -- 明文（加密存储）
    name_display    VARCHAR(32),                    -- 脱敏展示名，如"张*三"
    gender          VARCHAR(4) CHECK (gender IN ('男','女')),
    birth_date      DATE,
    age             INTEGER,
    phone           VARCHAR(20),                    -- 患者手机号
    phone_alt       VARCHAR(20),                    -- 第二联系人（子女）手机号
    disease_type    VARCHAR(32) NOT NULL CHECK (disease_type IN ('hypertension','diabetes','elderly','mental')),
    diagnosis_date  DATE,                           -- 确诊日期
    course_years    INTEGER,                        -- 病程（年）
    allergy_history TEXT,                           -- 过敏史
    complication_history TEXT,                      -- 并发症史
    current_medications TEXT,                       -- 当前医嘱用药（HIS同步）
    managing_doctor VARCHAR(64),                    -- 管床医生
    health_center   VARCHAR(128),                   -- 所属村卫生室/社区站
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uk_patient_id_disease UNIQUE (patient_id, disease_type)
);

COMMENT ON TABLE patients IS '患者基础档案表';
COMMENT ON COLUMN patients.phone_alt IS '第二联系人手机号，用于老年人短信未点击时的降级触达';

-- ============================================
-- 2. 随访任务表（含状态机 CHECK 约束）
-- ============================================
CREATE TABLE followup_tasks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         VARCHAR(64) NOT NULL,           -- 随访任务ID
    patient_id      UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    disease_type    VARCHAR(32) NOT NULL,
    followup_round  INTEGER NOT NULL DEFAULT 1,     -- 第几次随访（年内）
    plan_date       DATE NOT NULL,                  -- 计划随访日期
    actual_date     DATE,                           -- 实际完成日期
    channel         VARCHAR(16) CHECK (channel IN ('sms','phone','face','wechat','h5')),
    status          VARCHAR(16) NOT NULL CHECK (
        status IN ('created','scheduled','sms_sent','opened','in_progress','completed','expired','lost','cancelled')
    ),
    token           VARCHAR(256),                   -- JWT一次性Token（使用后失效）
    token_expired   BOOLEAN DEFAULT FALSE,          -- Token是否已失效
    lost_reason     TEXT,                           -- 失访原因
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    sent_at         TIMESTAMP WITH TIME ZONE,       -- 短信发送时间
    opened_at       TIMESTAMP WITH TIME ZONE,       -- 链接点击时间
    completed_at    TIMESTAMP WITH TIME ZONE,       -- 完成时间
    CONSTRAINT uk_task_id UNIQUE (task_id)
);

COMMENT ON TABLE followup_tasks IS '随访任务表，状态机：created→scheduled→sms_sent→opened→in_progress→completed（旁路终态：expired/lost/cancelled）';

-- ============================================
-- 3. 随访记录表
-- ============================================
CREATE TABLE followup_records (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id               VARCHAR(64) NOT NULL,
    task_id                 VARCHAR(64) NOT NULL REFERENCES followup_tasks(task_id),
    patient_id              UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    disease_type            VARCHAR(32) NOT NULL,
    followup_round          INTEGER NOT NULL,
    followup_date           DATE NOT NULL,
    followup_method         VARCHAR(16) CHECK (followup_method IN ('门诊','家庭','电话','自助','h5')),
    -- 结构化数据（JSONB，由schema.json驱动）
    structured_data         JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- 元数据
    source                  VARCHAR(32) NOT NULL CHECK (source IN ('self_reported','proxy_family','proxy_doctor','proxy_nurse')),
    proxy                   BOOLEAN NOT NULL DEFAULT FALSE,
    ai_raw                  JSONB,                  -- AI原始抽取结果（留痕）
    human_final             JSONB,                  -- 人工确认后结果
    completeness_score      INTEGER CHECK (completeness_score BETWEEN 0 AND 100),
    confidence              FLOAT CHECK (confidence BETWEEN 0.0 AND 1.0),
    -- 状态
    status                  VARCHAR(16) NOT NULL CHECK (
        status IN ('draft','submitted','confirmed','synced','returned')
    ),
    -- 分类与预警
    followup_class          VARCHAR(32),            -- 本次随访分类（C类计算）
    risk_level              VARCHAR(8) CHECK (risk_level IN ('green','yellow','red')),
    alert_ids               TEXT[],                 -- 关联的预警ID
    -- 时间戳
    created_at              TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    submitted_at            TIMESTAMP WITH TIME ZONE,
    confirmed_at            TIMESTAMP WITH TIME ZONE,
    synced_at               TIMESTAMP WITH TIME ZONE,
    updated_at              TIMESTAMP WITH TIME ZONE,
    CONSTRAINT uk_record_id UNIQUE (record_id)
);

COMMENT ON TABLE followup_records IS '随访记录表，状态机：draft→submitted→confirmed→synced（分支：submitted→returned→draft）';
COMMENT ON COLUMN followup_records.structured_data IS '由schema.json驱动的结构化字段，C类字段由系统计算';
COMMENT ON COLUMN followup_records.ai_raw IS 'AI原始抽取结果，用于审计留痕';
COMMENT ON COLUMN followup_records.human_final IS '人工确认后的最终结果';

-- ============================================
-- 4. 预警记录表
-- ============================================
CREATE TABLE alerts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id        VARCHAR(64) NOT NULL,
    record_id       VARCHAR(64) NOT NULL REFERENCES followup_records(record_id),
    task_id         VARCHAR(64) NOT NULL,
    patient_id      UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    disease_type    VARCHAR(32) NOT NULL,
    level           VARCHAR(8) NOT NULL CHECK (level IN ('red','yellow','green')),
    rule_id         VARCHAR(32) NOT NULL,          -- 命中的规则ID
    rule_name       VARCHAR(128),
    trigger_fields  JSONB,                         -- 触发字段及值
    suggestion      TEXT,                          -- AI建议（RAG生成）
    suggestion_src  TEXT,                          -- 建议出处（指南条款）
    status          VARCHAR(16) NOT NULL CHECK (
        status IN ('created','notified','assigned','handling','resolved','ignored','escalated')
    ),
    notified_at     TIMESTAMP WITH TIME ZONE,
    assigned_to     VARCHAR(64),                   -- 分配处理人
    handled_by      VARCHAR(64),                   -- 实际处理人
    handled_at      TIMESTAMP WITH TIME ZONE,
    handle_action   VARCHAR(64),                   -- 处理动作
    handle_note     TEXT,                          -- 处理备注
    escalated_to    VARCHAR(64),                   -- 升级上报人
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at      TIMESTAMP WITH TIME ZONE,
    CONSTRAINT uk_alert_id UNIQUE (alert_id)
);

COMMENT ON TABLE alerts IS '预警记录表，状态机：created→notified→assigned→handling→resolved|ignored|escalated';
COMMENT ON COLUMN alerts.level IS 'red级不允许ignored，只能resolved或escalated';

-- ============================================
-- 5. 差分日志表（审计留痕）
-- ============================================
CREATE TABLE diff_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    record_id       VARCHAR(64) NOT NULL REFERENCES followup_records(record_id) ON DELETE CASCADE,
    field_key       VARCHAR(64) NOT NULL,
    ai_value        JSONB,                         -- AI原始值
    human_value     JSONB,                         -- 人工修改值
    operator        VARCHAR(64) NOT NULL,          -- 操作人
    operation       VARCHAR(16) NOT NULL CHECK (operation IN ('create','modify','confirm','reject','sync')),
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

COMMENT ON TABLE diff_logs IS '差分日志表，记录AI抽取与人工确认的差异，用于审计追溯';

-- ============================================
-- 6. 短信日志表（Mock实现）
-- ============================================
CREATE TABLE sms_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id         VARCHAR(64) NOT NULL,
    patient_id      UUID NOT NULL REFERENCES patients(id) ON DELETE CASCADE,
    phone           VARCHAR(20) NOT NULL,
    template        VARCHAR(256),                  -- 短信模板
    short_link      VARCHAR(256),                  -- 短链接
    sent_at         TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    simulated_recv  BOOLEAN DEFAULT FALSE,         -- Mock：模拟接收
    clicked         BOOLEAN DEFAULT FALSE,         -- Mock：模拟点击
    clicked_at      TIMESTAMP WITH TIME ZONE,
    status          VARCHAR(16) DEFAULT 'sent' CHECK (status IN ('sent','delivered','clicked','failed'))
);

COMMENT ON TABLE sms_logs IS '短信日志表，MVP阶段为Mock实现，写数据库不接真实服务商';

-- ============================================
-- 7. 审计日志表
-- ============================================
CREATE TABLE audit_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         VARCHAR(64),                   -- 操作人ID（匿名化）
    user_role       VARCHAR(32),                   -- 角色
    action          VARCHAR(64) NOT NULL,          -- 操作类型
    resource_type   VARCHAR(32),                   -- 资源类型
    resource_id     VARCHAR(64),                   -- 资源ID
    ip_address      INET,
    user_agent      VARCHAR(256),
    request_body    JSONB,                         -- 请求参数（脱敏）
    response_body   JSONB,                         -- 响应结果（脱敏）
    created_at      TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

COMMENT ON TABLE audit_logs IS '审计日志表，全操作留痕，不可篡改';

-- ============================================
-- 索引
-- ============================================
CREATE INDEX idx_tasks_patient ON followup_tasks(patient_id);
CREATE INDEX idx_tasks_status ON followup_tasks(status) WHERE status NOT IN ('completed','expired','lost','cancelled');
CREATE INDEX idx_tasks_plan_date ON followup_tasks(plan_date);
CREATE INDEX idx_records_patient ON followup_records(patient_id);
CREATE INDEX idx_records_task ON followup_records(task_id);
CREATE INDEX idx_records_status ON followup_records(status);
CREATE INDEX idx_records_date ON followup_records(followup_date);
CREATE INDEX idx_alerts_level ON alerts(level, status) WHERE level IN ('red','yellow');
CREATE INDEX idx_alerts_patient ON alerts(patient_id);
CREATE INDEX idx_sms_logs_task ON sms_logs(task_id);
CREATE INDEX idx_audit_action ON audit_logs(action, created_at);

-- ============================================
-- 视图：今日任务看板
-- ============================================
CREATE VIEW v_today_tasks AS
SELECT
    t.task_id,
    t.patient_id,
    p.name_display,
    p.disease_type,
    t.followup_round,
    t.plan_date,
    t.channel,
    t.status,
    r.risk_level,
    r.followup_class,
    r.structured_data->>'sbp' AS last_sbp,
    r.structured_data->>'dbp' AS last_dbp,
    CASE
        WHEN t.status = 'completed' THEN '已完成'
        WHEN t.status = 'in_progress' THEN '进行中'
        WHEN t.status = 'expired' THEN '逾期'
        WHEN t.status = 'lost' THEN '失访'
        WHEN t.status = 'sms_sent' AND NOW() - t.sent_at > INTERVAL '72 hours' THEN '失访'
        ELSE '待随访'
    END AS status_text,
    CASE
        WHEN r.risk_level = 'red' THEN '高危'
        WHEN r.risk_level = 'yellow' THEN '中危'
        ELSE '正常'
    END AS risk_text
FROM followup_tasks t
JOIN patients p ON t.patient_id = p.id
LEFT JOIN LATERAL (
    SELECT risk_level, followup_class, structured_data
    FROM followup_records
    WHERE task_id = t.task_id
    ORDER BY created_at DESC
    LIMIT 1
) r ON true
WHERE t.plan_date = CURRENT_DATE
   OR (t.status IN ('in_progress') AND t.plan_date <= CURRENT_DATE);

COMMENT ON VIEW v_today_tasks IS '今日任务看板视图，供医生端首页使用';

-- ============================================
-- 视图：患者趋势数据
-- ============================================
CREATE VIEW v_patient_trend AS
SELECT
    patient_id,
    followup_date,
    structured_data->>'sbp' AS sbp,
    structured_data->>'dbp' AS dbp,
    structured_data->>'bmi' AS bmi,
    followup_class
FROM followup_records
WHERE disease_type = 'hypertension'
  AND status = 'confirmed'
ORDER BY patient_id, followup_date DESC;

COMMENT ON VIEW v_patient_trend IS '患者血压趋势视图，供趋势图使用';

-- ============================================
-- 数据保留策略注释
-- ============================================
-- 过程数据（录音、对话原文）：默认保留6个月，到期自动清理
-- 结构化随访记录、预警与处置记录：按病历管理要求长期保留
-- 审计日志：保留3年

-- ============================================
-- 枚举类型（可选，PostgreSQL 11+ 支持）
-- ============================================
-- 如果需要使用自定义枚举类型替代CHECK约束，可执行：
-- CREATE TYPE followup_status AS ENUM ('created','scheduled','sms_sent','opened','in_progress','completed','expired','lost','cancelled');
-- CREATE TYPE record_status AS ENUM ('draft','submitted','confirmed','synced','returned');
-- CREATE TYPE alert_status AS ENUM ('created','notified','assigned','handling','resolved','ignored','escalated');
-- CREATE TYPE alert_level AS ENUM ('red','yellow','green');
