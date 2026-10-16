package sync

import (
	"github.com/gin-gonic/gin"

	"MedFollow/internal/platform/response"
)

// Trigger 手动触发一轮患者数据同步（ADMIN）
func Trigger(s *Syncer) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := s.Run(c.Request.Context())
		if err != nil {
			response.AbortErr(c, 500, 50001, "sync failed")
			return
		}
		response.OK(c, res)
	}
}
