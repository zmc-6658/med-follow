package middleware

import (
	"time"

	"github.com/golang-jwt/jwt/v5"

	"MedFollow/internal/model"
)

const (
	RoleAdmin    = "ROLE_ADMIN"    // 系统管理员
	RoleDirector = "ROLE_DIRECTOR" // 科室主任
	RoleDoctor   = "ROLE_DOCTOR"   // 公卫医生
	RoleNurse    = "ROLE_NURSE"    // 随访护士
	RoleAuditor  = "ROLE_AUDITOR"  // 质控审核员
	RolePatient  = "ROLE_PATIENT"  // 患者端不走 RBAC，仅占位
)

const (
	PermTaskView    = "task:view"     // 查看随访任务
	PermTaskWrite   = "task:write"    // 开始/完成随访、表单编辑
	PermPatientView = "patient:view"  // 查看患者档案
	PermRecordView  = "record:view"   // 查看随访记录
	PermHISSubmit   = "his:submit"    // 提交随访至 HIS
	PermAlertHandle = "alert:handle"  // 处理预警/异常
	PermReportView  = "report:view"   // 质控报表
	PermExportData  = "export:data"   // 数据导出（需审批）
	PermStaffManage = "staff:manage"  // 人员管理
	PermAuditView   = "audit:view"    // 审计日志
	PermSysConfig   = "system:config" // 系统配置
	PermUserManage  = "user:manage"   // 角色管理
)

// 角色权限表，含继承展开（DIRECTOR→DOCTOR，AUDITOR→DIRECTOR，PRD 10.2）
var rolePermissions = map[string][]string{
	RoleDoctor: {PermTaskView, PermTaskWrite, PermPatientView, PermRecordView, PermHISSubmit, PermAlertHandle},
	RoleNurse:  {PermTaskView, PermTaskWrite, PermPatientView, PermRecordView, PermHISSubmit, PermAlertHandle},
	RoleAdmin:  {PermSysConfig, PermUserManage, PermAuditView},
}

func init() {
	rolePermissions[RoleDirector] = merge(rolePermissions[RoleDoctor], PermReportView, PermStaffManage, PermExportData)
	rolePermissions[RoleAuditor] = merge(rolePermissions[RoleDirector], PermReportView, PermExportData)
}

func merge(perms []string, extra ...string) []string {
	set := make(map[string]struct{}, len(perms)+len(extra))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	for _, p := range extra {
		set[p] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	return out
}

func HasPermission(role, perm string) bool {
	for _, p := range rolePermissions[role] {
		if p == perm {
			return true
		}
	}
	return false
}

// Claims JWT 载荷（PRD 10.5.2：sub/role/tenant_id/org_id/dept_id/exp/jti）
type Claims struct {
	Role     string `json:"role"`
	Username string `json:"username"`
	TenantID string `json:"tenant_id"`
	OrgID    string `json:"org_id"`
	DeptID   string `json:"dept_id"`
	jwt.RegisteredClaims
}

func (c *Claims) ToUserContext() *model.UserContext {
	return &model.UserContext{
		UserID:   c.Subject,
		Username: c.Username,
		Role:     c.Role,
		TenantID: c.TenantID,
		OrgID:    c.OrgID,
		DeptID:   c.DeptID,
		JTI:      c.ID,
	}
}

func NewClaims(uc *model.UserContext, ttl time.Duration) *Claims {
	now := time.Now()
	return &Claims{
		Role:     uc.Role,
		Username: uc.Username,
		TenantID: uc.TenantID,
		OrgID:    uc.OrgID,
		DeptID:   uc.DeptID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uc.UserID,
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uc.JTI,
		},
	}
}
