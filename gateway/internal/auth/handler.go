package auth

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"MedFollow/internal/config"
	"MedFollow/internal/middleware"
	"MedFollow/internal/platform/response"
)

// 认证域：登录 + 当前用户。测试环境用配置文件的用户信息
func mockUsers() map[string]config.DemoUser {
	users := make(map[string]config.DemoUser, len(config.Get().DemoUsers))
	for _, u := range config.Get().DemoUsers {
		users[u.Username] = u
	}
	return users
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginData struct {
	Token       string                `json:"token"`
	TokenType   string                `json:"token_type"`
	ExpiresIn   int64                 `json:"expires_in"`
	UserContext *middleware.UserContext `json:"user_context"`
}

func Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, response.Response{Code: 40000, Message: "username and password are required"})
		return
	}

	u, ok := mockUsers()[req.Username]
	// 不区分「用户不存在/密码错误」，防用户名枚举
	if !ok || u.Password != req.Password {
		c.JSON(http.StatusUnauthorized, response.Response{Code: 40102, Message: "invalid username or password"})
		return
	}

	cfg := config.Get()
	ttl := cfg.JWT.AccessTTL
	uc := &middleware.UserContext{
		UserID:   u.UserID,
		Username: u.Username,
		Role:     u.Role,
		TenantID: cfg.Defaults.TenantID,
		OrgID:    cfg.Defaults.OrgID,
		DeptID:   u.DeptID,
		JTI:      newJTI(),
	}

	token, err := jwtWithClaims(middleware.NewClaims(uc, ttl))
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Response{Code: 50000, Message: "failed to issue token"})
		return
	}

	response.OK(c, loginData{
		Token:       token,
		TokenType:   "Bearer",
		ExpiresIn:   int64(ttl / time.Second),
		UserContext: uc,
	})
}

func Me(c *gin.Context) {
	response.OK(c, middleware.GetUserContext(c))
}
