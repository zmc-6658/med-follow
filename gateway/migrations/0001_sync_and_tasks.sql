  -- 0001_sync_and_tasks.sql
-- 数据同步（YAML Mock 数据源）+ 任务生成器 增量变更
-- 基线：public schema 已有 7 张业务表（patients/followup_tasks/followup_records/alerts/
--       diff_logs/audit_logs/sms_logs）及 2 个视图，本文件只做幂等增量变更。
-- 可重复执行（IF NOT EXISTS / DO 块幂等）。

-- ============ 1. patients 增列 ============
-- his_patient_id：HIS 幂等键（同步 upsert 依据）
ALTER TABLE patients ADD COLUMN IF NOT EXISTS his_patient_id varchar(64);
-- phone_source：电话来源 his=HIS同步 / manual=人工修正
ALTER TABLE patients ADD COLUMN IF NOT EXISTS phone_source varchar(8) NOT NULL DEFAULT 'his';

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uk_patients_his_patient_id') THEN
    ALTER TABLE patients ADD CONSTRAINT uk_patients_his_patient_id UNIQUE (his_patient_id);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'patients_phone_source_check') THEN
    ALTER TABLE patients ADD CONSTRAINT patients_phone_source_check
      CHECK (phone_source IN ('his', 'manual'));
  END IF;
END $$;

-- ============ 2. sync_state 单行同步状态表 ============
CREATE TABLE IF NOT EXISTS sync_state (
  id           smallint     PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  last_sync_at timestamptz  NOT NULL DEFAULT 'epoch',
  updated_at   timestamptz  NOT NULL DEFAULT now()
);
INSERT INTO sync_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING;

-- ============ 3. sync_log 同步批次日志 ============
CREATE TABLE IF NOT EXISTS sync_log (
  id            bigserial    PRIMARY KEY,
  batch_id      varchar(64)  NOT NULL,
  started_at    timestamptz  NOT NULL,
  finished_at   timestamptz,
  fetched       integer      NOT NULL DEFAULT 0,
  inserted      integer      NOT NULL DEFAULT 0,
  updated       integer      NOT NULL DEFAULT 0,
  skipped       integer      NOT NULL DEFAULT 0,
  error_summary text
);
CREATE INDEX IF NOT EXISTS idx_sync_log_batch ON sync_log (batch_id);

-- ============ 4. followup_tasks 增列与约束 ============
-- period_key：随访周期键（如 '2026Q4'），任务生成器幂等键组成部分
ALTER TABLE followup_tasks ADD COLUMN IF NOT EXISTS period_key varchar(16);

-- channel 补充 'manual'（医生手动创建任务）；原有取值保持不变
ALTER TABLE followup_tasks DROP CONSTRAINT IF EXISTS followup_tasks_channel_check;
ALTER TABLE followup_tasks ADD CONSTRAINT followup_tasks_channel_check
  CHECK (channel IN ('sms', 'phone', 'face', 'wechat', 'h5', 'manual'));

-- 同一患者同病种同周期仅一个任务（任务生成器幂等依据）
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'uk_tasks_patient_disease_period') THEN
    ALTER TABLE followup_tasks
      ADD CONSTRAINT uk_tasks_patient_disease_period UNIQUE (patient_id, disease_type, period_key);
  END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_tasks_period_key ON followup_tasks (period_key);
