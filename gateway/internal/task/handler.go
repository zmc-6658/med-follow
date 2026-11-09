package task

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/platform/response"
)

// Generate 手动触发任务生成（ADMIN）
func Generate(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		period, created, err := GenerateTasks(c.Request.Context(), pool, time.Now())
		if err != nil {
			response.AbortErr(c, 500, 50002, "generate tasks failed")
			return
		}
		response.OK(c, gin.H{"period": period, "created": created})
	}
}

// List 任务列表（PermTaskView）：查询时惰性执行 72h 未点击降级（无定时器）
func List(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if _, err := expireStaleTasks(ctx, pool); err != nil {
			response.AbortErr(c, 500, 50002, "expire stale tasks failed")
			return
		}
		items, err := listTasks(ctx, pool)
		if err != nil {
			response.AbortErr(c, 500, 50002, "list tasks failed")
			return
		}
		response.OK(c, gin.H{"tasks": items})
	}
}
