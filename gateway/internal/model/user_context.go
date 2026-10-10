package model

// JWT 解析出的用户上下文（PRD 10.5.2），鉴权通过后注入 gin.Context
type UserContext struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id"`
	OrgID    string `json:"org_id"`
	DeptID   string `json:"dept_id"`
	JTI      string `json:"jti"`
}
