package sms

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/config"
	"MedFollow/internal/middleware"
	"MedFollow/internal/platform/response"
	"MedFollow/internal/trace"
)

// 哨兵错误：请求错误返回 4xx 且不落 sms_logs（与服务商失败区分，后者 HTTP 200 + failed 行）
var (
	ErrInvalidPhone    = errors.New("invalid phone number")
	ErrUnknownTemplate = errors.New("unknown template id")
	ErrTaskNotSendable = errors.New("task not sendable")
)

// 状态机（PRD 附录C）：终态不可下发不可回退，禁止 scheduled 直接跳 completed
var terminalStatus = map[string]bool{"lost": true, "cancelled": true, "expired": true}

const (
	TemplateFollowup        = "pt_followup"         // 患者本人模板
	TemplateContactReminder = "pt_contact_reminder" // 24h 未点击的联系人二次提醒模板
)

var phoneRe = regexp.MustCompile(`^1[3-9][0-9]{9}$`)

// SendRequest 一条短信的发送请求；link 变量必含一次性 Token 短链；
// 失败注入：Vars 带 mock_fail=1 时本条返回失败（MVP mock，不接服务商）
type SendRequest struct {
	TaskID     string            // followup_tasks.task_id（sms_logs 外键，落库必需）
	PatientID  string            // patients.id（uuid）
	Phone      string            // 接收号：本人 phone 或联系人 phone_alt
	TemplateID string            // 白名单内，见 etc/config.yaml sms.templates
	Vars       map[string]string // 模板变量
}

// SendResult 一条短信的发送结果，成功失败都落库
type SendResult struct {
	Status string `json:"status"` // sent 或 failed
	LogID  string `json:"log_id"` // sms_logs.id
	Detail string `json:"detail"` // 失败原因，成功为空
}

// DispatchResult 下发编排结果（医生视角）
type DispatchResult struct {
	TaskID      string       `json:"task_id"`
	TaskStatus  string       `json:"task_status"`
	PatientSent bool         `json:"patient_sent"`
	Results     []SendResult `json:"results"`
	ShortLink   string       `json:"short_link,omitempty"`
	Message     string       `json:"message"`
}

// Send Mock 发送一条短信：白名单校验 → 模板渲染 → 落 sms_logs（组件视角，只管一条短信的生死，
// 不含业务编排；P2 接真实服务商时只改本函数）
func Send(ctx context.Context, pool *pgxpool.Pool, req SendRequest) SendResult {
	tmpl, ok := config.Get().SMS.Templates[req.TemplateID]
	if !ok {
		return SendResult{Status: "failed", Detail: ErrUnknownTemplate.Error()}
	}
	if !phoneRe.MatchString(req.Phone) {
		return SendResult{Status: "failed", Detail: ErrInvalidPhone.Error()}
	}

	// 服务商失败注入：落 failed 行（short_link 留空，Token 未有效投递不可被点击）
	if req.Vars["mock_fail"] == "1" {
		id, err := insertLog(ctx, pool, req.TaskID, req.PatientID, req.Phone, req.TemplateID, "", "failed")
		if err != nil {
			return SendResult{Status: "failed", Detail: err.Error()}
		}
		return SendResult{Status: "failed", LogID: id, Detail: "mock_fail 注入失败"}
	}

	content := render(tmpl, req.Vars)
	id, err := insertLog(ctx, pool, req.TaskID, req.PatientID, req.Phone, req.TemplateID, req.Vars["link"], "sent")
	if err != nil {
		return SendResult{Status: "failed", Detail: err.Error()}
	}
	_ = content // mock 不真实发送，渲染内容仅验证模板可渲染
	return SendResult{Status: "sent", LogID: id}
}

// Dispatch 医生下发短信：本人号码首次下发 / 已 sent・opened 的二次下发（新行 + 新 Token），
// sms_sent 超 24h 未点击时向第二联系人（phone_alt）追加联系人提醒。
// 可选 body {"vars":{"mock_fail":"1"}} 为 Mock 失败注入开关（P2 接真实服务商后移除）
func Dispatch(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		taskID := c.Param("id")

		var req struct {
			Vars map[string]string `json:"vars"`
		}
		_ = c.ShouldBindJSON(&req) // body 可选

		t, err := loadTask(ctx, pool, taskID)
		if errors.Is(err, pgx.ErrNoRows) {
			response.AbortErr(c, 404, 40400, "task not found")
			return
		}
		if err != nil {
			response.AbortErr(c, 500, 50002, "load task failed")
			return
		}
		if terminalStatus[t.Status] || t.Status == "created" {
			response.AbortErr(c, 409, 40900, ErrTaskNotSendable.Error())
			return
		}
		if !phoneRe.MatchString(t.Phone) {
			response.AbortErr(c, 400, 40001, ErrInvalidPhone.Error())
			return
		}

		// 二次提醒判定：sms_sent 且本人短信 24h 未点击 → 追加第二联系人转发
		remindAlt := t.Status == "sms_sent" && !t.Clicked && t.SentAt != nil &&
			time.Since(*t.SentAt) > 24*time.Hour && phoneRe.MatchString(t.PhoneAlt)

		base := config.Get().SMS.BaseURL
		var results []SendResult

		token, err := SignClickToken(taskID, t.PatientID)
		if err != nil {
			response.AbortErr(c, 500, 50002, "sign token failed")
			return
		}
		patient := Send(ctx, pool, SendRequest{
			TaskID: taskID, PatientID: t.PatientID, Phone: t.Phone, TemplateID: TemplateFollowup,
			Vars: mergeVars(t.Name, token, req.Vars),
		})
		results = append(results, patient)
		patientSent := patient.Status == "sent"

		if remindAlt {
			altToken, err := SignClickToken(taskID, t.PatientID)
			if err != nil {
				response.AbortErr(c, 500, 50002, "sign token failed")
				return
			}
			results = append(results, Send(ctx, pool, SendRequest{
				TaskID: taskID, PatientID: t.PatientID, Phone: t.PhoneAlt, TemplateID: TemplateContactReminder,
				Vars: mergeVars(t.Name, altToken, req.Vars),
			}))
		}

		res := DispatchResult{TaskID: taskID, TaskStatus: t.Status, PatientSent: patientSent, Results: results}

		if patientSent {
			if t.Status == "scheduled" {
				if err = advanceTaskStatus(ctx, pool, taskID, "scheduled", "sms_sent"); err != nil {
					response.AbortErr(c, 500, 50002, "advance task failed")
					return
				}
				if err = markSentAt(ctx, pool, taskID); err != nil {
					response.AbortErr(c, 500, 50002, "mark sent_at failed")
					return
				}
			}
			// 二次下发：已 sms_sent / opened 保持原状态，不改已 opened
			res.TaskStatus = "sms_sent"
			if t.Status == "opened" {
				res.TaskStatus = "opened"
			}
			res.ShortLink = base + "/sms/click?token=" + token
			res.Message = "短信已发送，请提醒患者查收短信并点击链接"
			if remindAlt {
				res.Message += "；已向第二联系人追加提醒"
			}

			// 埋点：标识统一 HashID，不出现明文
			trace.EmitEvent(ctx, trace.EventSmsSent, map[string]any{
				"task_id":    trace.HashID(taskID),
				"patient_id": trace.HashID(t.PatientID),
				"disease":    t.DiseaseType,
				"channel":    "sms",
			})
			// 审计（append-only）
			uc := middleware.GetUserContext(c)
			if err = insertAudit(ctx, pool, uc.UserID, uc.Role, taskID, c.ClientIP()); err != nil {
				response.AbortErr(c, 500, 50002, "write audit failed")
				return
			}
		} else {
			// 服务商失败：HTTP 200，任务停留原状态，医生当下手动兜底（不等 24h 降级）
			res.Message = "短信发送失败（" + patient.Detail + "），请通过电话等方式手动随访兜底"
		}

		response.OK(c, res)
	}
}

// Click 患者点短链（公开路由，无 JWT）：校验 Token → 回写 clicked → sms_sent 推进 opened
func Click(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		token := c.Query("token")

		claims, err := ParseClickToken(token)
		if err != nil {
			response.AbortErr(c, 400, 40002, "invalid or expired token")
			return
		}
		ok, err := markClicked(ctx, pool, token)
		if err != nil {
			response.AbortErr(c, 500, 50002, "mark clicked failed")
			return
		}
		if ok {
			// 首次点击：sms_sent 推进 opened，埋一次 link_clicked
			if err = advanceTaskStatus(ctx, pool, claims.TaskID, "sms_sent", "opened"); err != nil {
				response.AbortErr(c, 500, 50002, "advance task failed")
				return
			}
			trace.EmitEvent(ctx, trace.EventLinkClicked, map[string]any{
				"task_id":    trace.HashID(claims.TaskID),
				"patient_id": trace.HashID(claims.PatientID),
			})
		} else {
			// 48h 内重复点击：幂等重开（患者误触/退出后重进），状态不推进、不重复埋点；
			// 无对应日志行 = Token 从未有效投递过，仍视为无效
			exists, err := sentLogExists(ctx, pool, token)
			if err != nil {
				response.AbortErr(c, 500, 50002, "check log failed")
				return
			}
			if !exists {
				response.AbortErr(c, 400, 40003, "token already used or invalid")
				return
			}
		}
		response.OK(c, gin.H{"task_id": claims.TaskID, "status": "opened"})
	}
}

// render 模板渲染：{{key}} 替换
func render(tmpl string, vars map[string]string) string {
	out := tmpl
	for k, v := range vars {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

// mergeVars 组装模板变量：name + link，外部注入 vars（失败注入）覆盖同名键
func mergeVars(name, token string, extra map[string]string) map[string]string {
	vars := map[string]string{
		"name": name,
		"link": config.Get().SMS.BaseURL + "/sms/click?token=" + token,
	}
	for k, v := range extra {
		vars[k] = v
	}
	return vars
}
