package task

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// GenerateTasks 按病种+周期生成随访任务（编排），SQL 在 store.go
// 生成规则（PRD 4.7 M1）：高血压每季度 1 次；本阶段仅高血压，新增病种时在此扩展
func GenerateTasks(ctx context.Context, pool *pgxpool.Pool, now time.Time) (period string, created int, err error) {
	period = PeriodKey(now)

	created, err = insertQuarterlyTasks(ctx, pool, period, nextWorkingDay(now).Format("2006-01-02"), strconv.Itoa(now.Year()))
	if err != nil {
		return period, 0, err
	}

	if err = promoteDueScheduled(ctx, pool); err != nil {
		return period, created, err
	}

	log.Printf("[task] period=%s created=%d", period, created)
	return period, created, nil
}

// PeriodKey 计算所属季度键，如 2026-10 → "2026Q4"
func PeriodKey(t time.Time) string {
	return fmt.Sprintf("%dQ%d", t.Year(), (int(t.Month())-1)/3+1)
}

// nextWorkingDay 下一个工作日（跳过周末），作为 plan_date
func nextWorkingDay(t time.Time) time.Time {
	d := t.AddDate(0, 0, 1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
