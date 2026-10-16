package task

import (
	"context"

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
