package route

import (
	"github.com/gin-gonic/gin"

	"MedFollow/internal/handler"
	"MedFollow/internal/middleware"
)

func Setup(r *gin.Engine) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	auth := r.Group("/api/v1/auth")
	{
		auth.POST("/login", handler.Login)
		auth.GET("/me", middleware.AuthRequired(), handler.Me)
	}

	// 业务接口：AuthRequired（401）→ RequirePermission（403）
	api := r.Group("/api/v1", middleware.AuthRequired())
	{
		api.GET("/tasks", middleware.RequirePermission(middleware.PermTaskView), handler.UserContextHandler(middleware.PermTaskView))
		api.GET("/reports/quality", middleware.RequirePermission(middleware.PermReportView), handler.UserContextHandler(middleware.PermReportView))
		api.GET("/audit-logs", middleware.RequirePermission(middleware.PermAuditView), handler.UserContextHandler(middleware.PermAuditView))
		api.GET("/admin/config", middleware.RequirePermission(middleware.PermSysConfig), handler.UserContextHandler(middleware.PermSysConfig))
	}
}
