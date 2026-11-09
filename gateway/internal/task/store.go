package task

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 本域全部 SQL：随访任务的季度生成与状态推进

// insertQuarterlyTasks 为全部高血压患者插入本季度任务，靠唯一约束幂等；返回新增条数
// channel：患者本人手机号合法走 sms，否则 manual（无手机/仅第二联系人/格式坏 → 样本②③④）
// 轮次：当年该患者该病种已有任务数 + 1（count(*) 不会为 NULL，无需 COALESCE 兜底）
func insertQuarterlyTasks(ctx context.Context, pool *pgxpool.Pool, period, planDate, year string) (int, error) {
	rows, err := pool.Query(ctx, `
		INSERT INTO followup_tasks
		  (task_id, patient_id, disease_type, followup_round, plan_date, channel, status, period_key)
		SELECT 'FT-' || p.patient_id || '-' || $1,
		       p.id, p.disease_type,
		       (SELECT count(*) + 1 FROM followup_tasks t
		         WHERE t.patient_id = p.id
		           AND t.disease_type = p.disease_type
		           AND t.period_key LIKE $3 || '%'),
		       $2::date,
		       CASE WHEN p.phone ~ '^1[3-9][0-9]{9}$' THEN 'sms' ELSE 'manual' END,
		       'created', $1
		FROM patients p
		WHERE p.disease_type = 'hypertension'
		ON CONFLICT (patient_id, disease_type, period_key) DO NOTHING
		RETURNING task_id`, period, planDate, year)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	created := 0
	for rows.Next() {
		created++
	}
	return created, rows.Err()
}

// promoteDueScheduled 排期推进：plan_date 已到的 created 任务 → scheduled（PRD 附录C created→scheduled）
func promoteDueScheduled(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`UPDATE followup_tasks SET status = 'scheduled'
		 WHERE status = 'created' AND plan_date <= current_date`)
	return err
}

// expireStaleTasks 惰性降级（不写定时器，任务列表查询时顺带执行）：
// sms_sent 且无任何点击超 72h → 置 lost（终态），返回推进条数
func expireStaleTasks(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE followup_tasks SET status = 'lost', lost_reason = 'sent后72h未点击失访', updated_at = now()
		WHERE status = 'sms_sent' AND sent_at < now() - interval '72 hours'
		  AND NOT EXISTS (SELECT 1 FROM sms_logs l WHERE l.task_id = followup_tasks.task_id AND l.clicked)`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// TaskListItem 任务列表行
type TaskListItem struct {
	TaskID      string     `json:"task_id"`
	PatientID   string     `json:"patient_id"`
	PatientName string     `json:"patient_name"`
	DiseaseType string     `json:"disease_type"`
	Status      string     `json:"status"`
	PlanDate    string     `json:"plan_date"`
	SentAt      *time.Time `json:"sent_at"`
	CanRemind   bool       `json:"can_remind"` // sms_sent 超 24h 未点击，可向联系人二次提醒
}

// listTasks 全量任务列表（MVP 不分页，数据量为季度任务数）
func listTasks(ctx context.Context, pool *pgxpool.Pool) ([]TaskListItem, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.task_id, p.patient_id, COALESCE(p.name_display, p.name), t.disease_type, t.status,
		       t.plan_date::text, t.sent_at,
		       t.status = 'sms_sent' AND t.sent_at < now() - interval '24 hours'
		         AND NOT EXISTS (SELECT 1 FROM sms_logs l
		                         WHERE l.task_id = t.task_id AND l.clicked) AS can_remind
		FROM followup_tasks t JOIN patients p ON p.id = t.patient_id
		ORDER BY t.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []TaskListItem{}
	for rows.Next() {
		var it TaskListItem
		if err := rows.Scan(&it.TaskID, &it.PatientID, &it.PatientName, &it.DiseaseType,
			&it.Status, &it.PlanDate, &it.SentAt, &it.CanRemind); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}
