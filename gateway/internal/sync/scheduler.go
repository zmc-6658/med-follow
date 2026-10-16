package sync

import (
	"context"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/task"
)

// RunScheduler M1 管道编排：启动即执行一轮「同步 + 任务生成」，之后按 interval 定时执行
func RunScheduler(ctx context.Context, syncer *Syncer, pool *pgxpool.Pool, interval time.Duration) {
	run := func() {
		if _, err := syncer.Run(ctx); err != nil {
			log.Printf("[sync] batch failed: %v", err)
		}
		if _, _, err := task.GenerateTasks(ctx, pool, time.Now()); err != nil {
			log.Printf("[task] generate failed: %v", err)
		}
	}

	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			run()
		}
	}
}
