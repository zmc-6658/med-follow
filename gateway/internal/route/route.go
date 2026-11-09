package route

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/auth"
	"MedFollow/internal/middleware"
	"MedFollow/internal/platform/response"
	"MedFollow/internal/sms"
	"MedFollow/internal/sync"
	"MedFollow/internal/task"
)

func Setup(r *gin.Engine, syncer *sync.Syncer, pool *pgxpool.Pool) {
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	authGroup := r.Group("/api/v1/auth")
	{
		authGroup.POST("/login", auth.Login)
		authGroup.GET("/me", middleware.AuthRequired(), auth.Me)
	}

	// 公开路由：患者手机点短信短链，无登录 JWT，靠一次性 Token 自校验
	smsClick := r.Group("/sms")
	{
		smsClick.GET("/click", sms.Click(pool))
	}

	// 业务接口：AuthRequired（401）→ RequirePermission（403）
	api := r.Group("/api/v1", middleware.AuthRequired())
	{
		// 占位演示接口（真实域落地后逐个替换为域 handler）
		api.GET("/reports/quality", middleware.RequirePermission(middleware.PermReportView), demoContext(middleware.PermReportView))
		api.GET("/audit-logs", middleware.RequirePermission(middleware.PermAuditView), demoContext(middleware.PermAuditView))
		api.GET("/admin/config", middleware.RequirePermission(middleware.PermSysConfig), demoContext(middleware.PermSysConfig))

		// 数据同步与任务生成（M1，仅 ADMIN）
		api.POST("/admin/sync", middleware.RequirePermission(middleware.PermSysConfig), sync.Trigger(syncer))
		api.POST("/admin/tasks/generate", middleware.RequirePermission(middleware.PermSysConfig), task.Generate(pool))

		// 随访任务域：列表（task:view）/ 短信下发（sms:send，医生与继承角色）
		api.GET("/tasks", middleware.RequirePermission(middleware.PermTaskView), task.List(pool))
		api.POST("/tasks/:id/send", middleware.RequirePermission(middleware.PermSmsSend), sms.Dispatch(pool))
	}
}

// demoContext 返回注入的用户上下文，验证「鉴权通过 → 200 + 用户上下文 JSON」；
// 真实业务中数据层基于该上下文做行级过滤
func demoContext(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		response.OK(c, gin.H{
			"permission_checked": perm,
			"user_context":       middleware.GetUserContext(c),
		})
	}
}
