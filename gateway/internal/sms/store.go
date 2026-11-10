package sms

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 本包全部 SQL：sms_logs 落库回写、followup_tasks 状态机推进（复用前置态校验模式）

// insertLog 落一条短信日志，成功与失败都写（status: sent / failed）。
// 失败行 short_link 留空：失败短信的 Token 从未有效投递，不允许被点击匹配
func insertLog(ctx context.Context, pool *pgxpool.Pool, taskID string, patientID string,
	phone, template, shortLink, status string) (string, error) {
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO sms_logs (task_id, patient_id, phone, template, short_link, status, simulated_recv)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		taskID, patientID, phone, template, shortLink, status, status == "sent").Scan(&id)
	return id, err
}

// markClicked 按短链中的 Token 匹配未点击行回写，WHERE clicked=false 保证单次有效；
// 返回是否命中（0 行 = Token 已用/无效）
func markClicked(ctx context.Context, pool *pgxpool.Pool, token string) (bool, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE sms_logs SET clicked = true, clicked_at = now()
		WHERE clicked = false AND short_link LIKE '%' || $1`, token)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// revokeUnclickedLinks 二次下发时作废该任务此前未点击的短链（最新短信为准）：
// 清空 short_link 后 sentLogExists 匹配不到，旧 Token 自然失效；
// keepLogID 为本次新落的行；已点击的旧行保留，供误关页面后幂等重开
func revokeUnclickedLinks(ctx context.Context, pool *pgxpool.Pool, taskID, keepLogID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE sms_logs SET short_link='' WHERE task_id=$1 AND clicked=false AND status='sent' AND id<>$2`,
		taskID, keepLogID)
	return err
}

// sentLogExists 判断 Token 是否对应一条已投递的短信日志（幂等重开的准入校验）
func sentLogExists(ctx context.Context, pool *pgxpool.Pool, token string) (bool, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM sms_logs WHERE short_link LIKE '%' || $1`, token).Scan(&n)
	return n > 0, err
}

// advanceTaskStatus 状态机推进：WHERE 前置态校验，命中 0 行视为状态不符（不报错，由调用方定语义）
func advanceTaskStatus(ctx context.Context, pool *pgxpool.Pool, taskID, from, to string) error {
	extra := ""
	if to == "opened" {
		extra = ", opened_at = now()"
	}
	_, err := pool.Exec(ctx,
		`UPDATE followup_tasks SET status = $3, updated_at = now()`+extra+`
		 WHERE task_id = $1 AND status = $2`, taskID, from, to)
	return err
}

// markSentAt 首次下发成功回填 sent_at
func markSentAt(ctx context.Context, pool *pgxpool.Pool, taskID string) error {
	_, err := pool.Exec(ctx,
		`UPDATE followup_tasks SET sent_at = now(), updated_at = now()
		 WHERE task_id = $1 AND sent_at IS NULL`, taskID)
	return err
}

// TaskRow 下发编排所需的任务+患者快照
type TaskRow struct {
	TaskID      string
	Status      string
	PatientID   string // patients.id（uuid）
	DiseaseType string
	Name        string // 患者姓名（仅进短信内容，不进埋点）
	Phone       string
	PhoneAlt    string
	SentAt      *time.Time
	Clicked     bool // 本人号码最新一条短信是否已点击
}

// loadTask 取任务与患者快照（本人/第二联系人号码、最新短信点击状态）
func loadTask(ctx context.Context, pool *pgxpool.Pool, taskID string) (*TaskRow, error) {
	row := &TaskRow{TaskID: taskID}
	err := pool.QueryRow(ctx, `
		SELECT t.status, p.id::text, p.disease_type, p.name, p.phone, p.phone_alt, t.sent_at,
		       COALESCE((SELECT l.clicked FROM sms_logs l
		                 WHERE l.task_id = t.task_id ORDER BY l.sent_at DESC LIMIT 1), false)
		FROM followup_tasks t JOIN patients p ON p.id = t.patient_id
		WHERE t.task_id = $1`, taskID).Scan(
		&row.Status, &row.PatientID, &row.DiseaseType, &row.Name, &row.Phone, &row.PhoneAlt,
		&row.SentAt, &row.Clicked)
	if err != nil {
		return nil, err
	}
	return row, nil
}

// insertAudit 审计留痕（append-only）：医生下发短信
func insertAudit(ctx context.Context, pool *pgxpool.Pool, userID, role, taskID, ip string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO audit_logs (user_id, user_role, action, resource_type, resource_id, ip_address)
		VALUES ($1, $2, 'send_sms', 'followup_task', $3, $4)`, userID, role, taskID, ip)
	return err
}
