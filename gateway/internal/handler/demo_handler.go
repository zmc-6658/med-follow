package handler

import (
	"github.com/gin-gonic/gin"

	"MedFollow/internal/middleware"
	"MedFollow/internal/model"
)

// 返回注入的用户上下文，验证「鉴权通过 → 200 + 用户上下文 JSON」；
// 真实业务中数据层基于该上下文做行级过滤
func UserContextHandler(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		model.OK(c, gin.H{
			"permission_checked": perm,
			"user_context":       middleware.GetUserContext(c),
		})
	}
}
