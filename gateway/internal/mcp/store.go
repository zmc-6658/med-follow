package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// 本域全部 SQL：MCP 工具对 patients / followup_tasks / followup_records / alerts / audit_logs 的最小读写

// queryPatient 查患者档案；返回列显式圈定，证件号类字段永不外泄（写域隔离的读侧约束）
func (s *Server) queryPatient(args json.RawMessage) (any, error) {
	var p struct {
		PatientID string `json:"patient_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.PatientID == "" {
		return nil, fmt.Errorf("patient_id required")
	}

	var out map[string]any
	err := s.pool.QueryRow(ctx(), `
		SELECT json_build_object(
			'patient_id',        patient_id,
			'name',              name,
			'gender',            gender,
			'birth_date',        birth_date,
			'age',               age,
			'phone',             phone,
			'disease_type',      disease_type,
			'diagnosis_date',    diagnosis_date,
			'current_medications', current_medications,
			'managing_doctor',   managing_doctor,
			'health_center',     health_center)
		FROM patients WHERE patient_id = $1`, p.PatientID).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("patient not found: %s", p.PatientID)
	}
	return out, err
}

// listTasks 查随访任务列表；status 为空串时不过滤
func (s *Server) listTasks(args json.RawMessage) (any, error) {
	var p struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, fmt.Errorf("invalid arguments")
	}

	var raw []byte
	err := s.pool.QueryRow(ctx(), `
		SELECT coalesce(json_agg(t), '[]'::json) FROM (
			SELECT task_id, disease_type, followup_round, plan_date, channel, status, period_key
			FROM followup_tasks
			WHERE ($1 = '' OR status = $1)
			ORDER BY plan_date, task_id
		) t`, p.Status).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var tasks []any
	if err := json.Unmarshal(raw, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// queryHistoryTrend 患者近 N 次随访记录的结构化关键值，最近一次在前；无记录返回空数组不报错
func (s *Server) queryHistoryTrend(args json.RawMessage) (any, error) {
	var p struct {
		PatientID string `json:"patient_id"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.PatientID == "" {
		return nil, fmt.Errorf("patient_id required")
	}
	if p.Limit <= 0 {
		p.Limit = 3
	}

	var raw []byte
	err := s.pool.QueryRow(ctx(), `
		SELECT coalesce(json_agg(t), '[]'::json) FROM (
			SELECT to_char(followup_date, 'YYYY-MM-DD') AS followup_date, structured_data
			FROM followup_records
			WHERE patient_id = (SELECT id FROM patients WHERE patient_id = $1)
			ORDER BY followup_date DESC
			LIMIT $2
		) t`, p.PatientID, p.Limit).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var records []any
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// saveFollowup AI 随访结果落草稿：status 恒为 'draft'、source 恒为 'ai'（写域隔离，
// 不接受任何入参覆盖状态，提交到 submitted 及以后必须走人工确认，MCP 无此工具）
func (s *Server) saveFollowup(args json.RawMessage) (any, error) {
	var p struct {
		TaskID         string          `json:"task_id"`
		StructuredData json.RawMessage `json:"structured_data"`
		AIRaw          json.RawMessage `json:"ai_raw"`
		Confidence     *float64        `json:"confidence"`
	}
	if err := json.Unmarshal(args, &p); err != nil || p.TaskID == "" {
		return nil, fmt.Errorf("task_id required")
	}
	structured := p.StructuredData
	if len(structured) == 0 {
		structured = json.RawMessage("{}")
	}

	var recordID string
	err := s.pool.QueryRow(ctx(), `
		INSERT INTO followup_records
		  (record_id, task_id, patient_id, disease_type, followup_round, followup_date,
		   structured_data, source, ai_raw, confidence, status)
		SELECT 'FR-' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 20),
		       t.task_id, t.patient_id, t.disease_type, t.followup_round, current_date,
		       $2::jsonb, 'ai', $3::jsonb, $4, 'draft'
		FROM followup_tasks t
		WHERE t.task_id = $1
		RETURNING record_id`,
		p.TaskID, structured, bytesOrNull(p.AIRaw), p.Confidence).Scan(&recordID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("task not found: %s", p.TaskID)
	}
	if err != nil {
		return nil, err
	}
	return map[string]string{"record_id": recordID}, nil
}

// pushAlert 任务级预警：record_id 恒为 NULL（对话实时预警，随访记录尚未生成），
// status 恒为 'open'；任务存在即可插入，不依赖是否有随访记录
func (s *Server) pushAlert(args json.RawMessage) (any, error) {
	var p struct {
		TaskID string `json:"task_id"`
		Level  string `json:"level"`
		RuleID string `json:"rule_id"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(args, &p); err != nil ||
		p.TaskID == "" || p.Level == "" || p.RuleID == "" {
		return nil, fmt.Errorf("task_id / level / rule_id required")
	}

	var alertID string
	err := s.pool.QueryRow(ctx(), `
		INSERT INTO alerts
		  (alert_id, record_id, task_id, patient_id, disease_type, level, rule_id, suggestion, status)
		SELECT 'AL-' || substr(replace(gen_random_uuid()::text, '-', ''), 1, 20),
		       NULL, t.task_id, t.patient_id, t.disease_type, $2, $3, $4, 'open'
		FROM followup_tasks t
		WHERE t.task_id = $1
		RETURNING alert_id`,
		p.TaskID, p.Level, p.RuleID, p.Reason).Scan(&alertID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("task not found: %s", p.TaskID)
	}
	if err != nil {
		return nil, err
	}
	return map[string]string{"alert_id": alertID}, nil
}

// insertAuditLog 写工具调用审计：caller 身份 + 工具名 + 入参 + 结果（PRD：工具调用可审计）
func insertAuditLog(pool *pgxpool.Pool, caller, tool string, args json.RawMessage, result any) error {
	resJSON, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO audit_logs (user_id, action, resource_type, resource_id, request_body, response_body)
		VALUES ($1, 'mcp_call', $2, $3, $4::jsonb, $5::jsonb)`,
		caller, tool, taskIDOf(args), bytesOrNull(args), resJSON)
	return err
}

// ---- 小工具 ----

func ctx() context.Context {
	return context.Background()
}

// bytesOrNull 空切片转 SQL NULL（jsonb 可空列不落空串）
func bytesOrNull(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

// taskIDOf 从写工具入参中取 task_id 作为审计 resource_id
func taskIDOf(args json.RawMessage) string {
	var p struct {
		TaskID string `json:"task_id"`
	}
	if json.Unmarshal(args, &p) == nil {
		return p.TaskID
	}
	return ""
}
