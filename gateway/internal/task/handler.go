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
