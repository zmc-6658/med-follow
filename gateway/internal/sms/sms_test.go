package sms

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"MedFollow/internal/config"
	"MedFollow/internal/middleware"
	"MedFollow/internal/task"
)

// 集成测试依赖本地 PostgreSQL（etc/config.yaml），并复用库中已生成的季度任务。
// 为可重复执行，每个用例先重置所用任务的状态与短信日志。

var pool *pgxpool.Pool

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Setenv("GATEWAY_CONFIG", "../../etc/config.yaml")
	config.MustLoad()

	var err error
	if pool, err = pgxpool.New(context.Background(), config.Get().Database.DSN()); err != nil {
		panic(err)
	}
	defer pool.Close()
	os.Exit(m.Run())
}

// resetTask 重置任务为 scheduled 并清空其短信日志（测试可重复执行）
func resetTask(t *testing.T, taskID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE followup_tasks SET status='scheduled', sent_at=NULL, opened_at=NULL, lost_reason=NULL
		 WHERE task_id=$1`, taskID); err != nil {
		t.Fatalf("reset task: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`DELETE FROM sms_logs WHERE task_id=$1`, taskID); err != nil {
		t.Fatalf("reset sms_logs: %v", err)
	}
}

func taskStatus(t *testing.T, taskID string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(),
		`SELECT status FROM followup_tasks WHERE task_id=$1`, taskID).Scan(&status); err != nil {
		t.Fatalf("query task status: %v", err)
	}
	return status
}

func logStats(t *testing.T, taskID string) (total int, clicked int, failed int) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		SELECT count(*), count(*) FILTER (WHERE clicked), count(*) FILTER (WHERE status='failed')
		FROM sms_logs WHERE task_id=$1`, taskID).Scan(&total, &clicked, &failed)
	if err != nil {
		t.Fatalf("query sms_logs: %v", err)
	}
	return
}

// invoke 以 gin 测试上下文调用 handler，返回响应体
func invoke(t *testing.T, h gin.HandlerFunc, method, path string, params gin.Params) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	c.Params = params
	h(c)

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse response %s: %v", w.Body.String(), err)
	}
	return w.Code, body
}

// dispatchAs 以医生身份调用下发接口，vars 为可选注入变量（失败注入）
func dispatchAs(t *testing.T, taskID string, vars map[string]string) (int, map[string]any) {
	t.Helper()
	h := Dispatch(pool)
	return func() (int, map[string]any) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		var bodyReader *strings.Reader
		if vars != nil {
			b, _ := json.Marshal(map[string]any{"vars": vars})
			bodyReader = strings.NewReader(string(b))
		} else {
			bodyReader = strings.NewReader("")
		}
		c.Request = httptest.NewRequest("POST", "/api/v1/tasks/"+taskID+"/send", bodyReader)
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "id", Value: taskID}}
		c.Set("user_context", &middleware.UserContext{UserID: "u1003", Role: "ROLE_DOCTOR"})
		h(c)

		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("parse response %s: %v", w.Body.String(), err)
		}
		return w.Code, body
	}()
}

// clickAs 公开组点击短链
func clickAs(t *testing.T, token string) (int, map[string]any) {
	t.Helper()
	return invoke(t, Click(pool), "GET", "/sms/click?token="+token,
		nil)
}

func dataMap(body map[string]any) map[string]any {
	d, _ := body["data"].(map[string]any)
	return d
}

func tokenFromLink(link string) string {
	return strings.SplitN(link, "token=", 2)[1]
}

// TestTokenSignParseRoundtrip Token 签发与校验
func TestTokenSignParseRoundtrip(t *testing.T) {
	raw, err := SignClickToken("FT-T1", "p-uuid")
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := ParseClickToken(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.TaskID != "FT-T1" || claims.PatientID != "p-uuid" || claims.Typ != TypSmsClick {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.ExpiresAt.Time.Sub(time.Now()) <= 47*time.Hour {
		t.Fatalf("exp should be ~48h, got %v", claims.ExpiresAt.Time)
	}
}

// TestTokenRejects 伪造签名 / 过期 / typ 不符均拒绝（A5 的 Token 层断言）
func TestTokenRejects(t *testing.T) {

	// typ 不符：同 secret 签发但 typ=login
	login := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"task_id": "FT-T1", "typ": "login", "exp": time.Now().Add(time.Hour).Unix(),
	})
	typMismatch, _ := login.SignedString(middleware.JWTSecret())

	// 过期
	expired := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"task_id": "FT-T1", "typ": TypSmsClick, "exp": time.Now().Add(-time.Hour).Unix(),
	})
	expiredRaw, _ := expired.SignedString(middleware.JWTSecret())

	// 伪造签名
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"task_id": "FT-T1", "typ": TypSmsClick, "exp": time.Now().Add(time.Hour).Unix(),
	})
	forgedRaw, _ := forged.SignedString([]byte("wrong-secret"))

	for name, tok := range map[string]string{"typ-mismatch": typMismatch, "expired": expiredRaw, "forged": forgedRaw, "empty": ""} {
		if _, err := ParseClickToken(tok); err == nil {
			t.Fatalf("%s token should be rejected", name)
		}
	}
}

// TestSendValidation 白名单与号码校验：请求错误不落库
func TestSendValidation(t *testing.T) {
	ctx := context.Background()

	res := Send(ctx, pool, SendRequest{TaskID: "FT-X", PatientID: "x", Phone: "13800138000", TemplateID: "pt_unknown"})
	if res.Status != "failed" || res.Detail != ErrUnknownTemplate.Error() {
		t.Fatalf("unknown template should fail: %+v", res)
	}

	res = Send(ctx, pool, SendRequest{TaskID: "FT-X", PatientID: "x", Phone: "12345", TemplateID: TemplateFollowup})
	if res.Status != "failed" || res.Detail != ErrInvalidPhone.Error() {
		t.Fatalf("invalid phone should fail: %+v", res)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sms_logs WHERE task_id='FT-X'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("request errors must not insert sms_logs, got %d rows (err=%v)", n, err)
	}
}

// TestSendMockFailInjection A6：失败注入落 failed 行，任务停留 scheduled，Message 含手动兜底提示
func TestSendMockFailInjection(t *testing.T) {
	const taskID = "FT-HIS000003-2026Q4"
	resetTask(t, taskID)

	code, body := dispatchAs(t, taskID, map[string]string{"mock_fail": "1"})
	if code != 200 {
		t.Fatalf("dispatch should return 200 on provider failure, got %d: %v", code, body)
	}
	d := dataMap(body)
	if d["task_status"] != "scheduled" {
		t.Fatalf("task should stay scheduled, got %v", d["task_status"])
	}
	if msg, _ := d["message"].(string); !strings.Contains(msg, "手动") || !strings.Contains(msg, "兜底") {
		t.Fatalf("message should contain manual fallback hint, got: %v", msg)
	}

	total, clicked, failed := logStats(t, taskID)
	if total != 1 || failed != 1 || clicked != 0 {
		t.Fatalf("expect 1 failed row, got total=%d failed=%d clicked=%d", total, clicked, failed)
	}
	if taskStatus(t, taskID) != "scheduled" {
		t.Fatalf("task status should stay scheduled")
	}
}

// TestDispatchClickFlow A1-A5：下发 → 点击 → 幂等重开 → 二次下发 → 非法 Token 无副作用
func TestDispatchClickFlow(t *testing.T) {
	const taskID = "FT-HIS000001-2026Q4"
	resetTask(t, taskID)

	// A1 下发成功
	code, body := dispatchAs(t, taskID, nil)
	if code != 200 {
		t.Fatalf("dispatch: %d %v", code, body)
	}
	d := dataMap(body)
	link, _ := d["short_link"].(string)
	if d["task_status"] != "sms_sent" || link == "" {
		t.Fatalf("expect task_status=sms_sent and short_link, got %v", d)
	}
	if taskStatus(t, taskID) != "sms_sent" {
		t.Fatalf("A1: task status should be sms_sent")
	}
	total, clicked, _ := logStats(t, taskID)
	if total != 1 || clicked != 0 {
		t.Fatalf("A1: expect 1 row unclicked, got total=%d clicked=%d", total, clicked)
	}

	// 审计落库
	var audits int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_logs WHERE action='send_sms' AND resource_id=$1`, taskID).Scan(&audits); err != nil || audits == 0 {
		t.Fatalf("A1: audit_logs should have send_sms row, got %d (err=%v)", audits, err)
	}

	token := tokenFromLink(link)

	// A2 点击短链
	code, body = clickAs(t, token)
	if code != 200 {
		t.Fatalf("A2 click: %d %v", code, body)
	}
	if taskStatus(t, taskID) != "opened" {
		t.Fatalf("A2: task status should be opened")
	}
	var clickedFlag bool
	var clickedAt *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT clicked, clicked_at FROM sms_logs WHERE task_id=$1 AND short_link LIKE '%'||$2`,
		taskID, token).Scan(&clickedFlag, &clickedAt); err != nil || !clickedFlag || clickedAt == nil {
		t.Fatalf("A2: log should be clicked with clicked_at (err=%v)", err)
	}

	// A3 Token 幂等重开：48h 内重复点击返回 200，状态与点击计数不变
	code, _ = clickAs(t, token)
	if code != 200 {
		t.Fatalf("A3: replay should be 200 (idempotent reopen), got %d", code)
	}
	if taskStatus(t, taskID) != "opened" {
		t.Fatalf("A3: task status should stay opened")
	}
	if _, clicked, _ = logStats(t, taskID); clicked != 1 {
		t.Fatalf("A3: clicked count should stay 1, got %d", clicked)
	}

	// A4 二次下发：新行 + 新 Token，不改已 opened 状态
	code, body = dispatchAs(t, taskID, nil)
	if code != 200 {
		t.Fatalf("A4 redispatch: %d %v", code, body)
	}
	if d = dataMap(body); d["task_status"] != "opened" {
		t.Fatalf("A4: re-dispatch should keep opened, got %v", d["task_status"])
	}
	total, _, _ = logStats(t, taskID)
	if total != 2 {
		t.Fatalf("A4: expect 2 log rows, got %d", total)
	}
	if d = dataMap(body); tokenFromLink(d["short_link"].(string)) == token {
		t.Fatalf("A4: new token must differ from old")
	}
	newToken := tokenFromLink(d["short_link"].(string))

	if code, _ = clickAs(t, newToken); code != 200 {
		t.Fatalf("A4: new token should click ok, got %d", code)
	}
	if taskStatus(t, taskID) != "opened" {
		t.Fatalf("A4: status should stay opened after second click")
	}
	// 旧 Token 已点击过，作废只针对未点击行：仍可幂等重开
	if _, clickedAfterOld, _ := logStats(t, taskID); clickedAfterOld != 2 {
		t.Fatalf("A4: clicked count should be 2 before old-token click, got %d", clickedAfterOld)
	}
	if code, _ = clickAs(t, token); code != 200 {
		t.Fatalf("A4: old token should reopen idempotently, got %d", code)
	}
	if taskStatus(t, taskID) != "opened" {
		t.Fatalf("A4: status should stay opened after old-token reopen")
	}
	if _, clickedAfterOld, _ := logStats(t, taskID); clickedAfterOld != 2 {
		t.Fatalf("A4: old-token reopen must not change clicked count, got %d", clickedAfterOld)
	}

	// A5 非法 Token：无任何 DB 变化
	beforeTotal, beforeClicked, _ := logStats(t, taskID)
	statusBefore := taskStatus(t, taskID)
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"task_id": taskID, "typ": TypSmsClick, "exp": time.Now().Add(time.Hour).Unix(),
	})
	forgedRaw, _ := forged.SignedString([]byte("wrong-secret"))
	for _, bad := range []string{forgedRaw, "not-a-jwt", ""} {
		if code, _ = clickAs(t, bad); code < 400 || code >= 500 {
			t.Fatalf("A5: bad token should be 4xx, got %d", code)
		}
	}
	afterTotal, afterClicked, _ := logStats(t, taskID)
	if afterTotal != beforeTotal || afterClicked != beforeClicked || taskStatus(t, taskID) != statusBefore {
		t.Fatalf("A5: invalid tokens must not change any state")
	}
}

// TestRedispatchRevokesUnclicked 二次下发作废未点击的旧链接：
// 旧 Token 变 400，新 Token 可点击；已点击过的旧 Token 幂等重开（在 A4 中覆盖）
func TestRedispatchRevokesUnclicked(t *testing.T) {
	const taskID = "FT-HIS000002-2026Q4"
	resetTask(t, taskID)

	// 第一次下发，不点击
	_, body := dispatchAs(t, taskID, nil)
	oldToken := tokenFromLink(dataMap(body)["short_link"].(string))

	// 第二次下发
	code, body := dispatchAs(t, taskID, nil)
	if code != 200 {
		t.Fatalf("redispatch: %d %v", code, body)
	}
	newToken := tokenFromLink(dataMap(body)["short_link"].(string))
	if newToken == oldToken {
		t.Fatalf("tokens must differ")
	}

	// 未点击的旧 Token 已作废：400 且无 DB 变化
	total, clicked, _ := logStats(t, taskID)
	statusBefore := taskStatus(t, taskID)
	if code, _ = clickAs(t, oldToken); code < 400 || code >= 500 {
		t.Fatalf("stale unclicked token should be revoked (4xx), got %d", code)
	}
	if afterTotal, afterClicked, _ := logStats(t, taskID); afterTotal != total || afterClicked != clicked {
		t.Fatalf("stale token click must not change logs, got total=%d clicked=%d", afterTotal, afterClicked)
	}
	if taskStatus(t, taskID) != statusBefore {
		t.Fatalf("stale token click must not change task status")
	}

	// 新 Token 正常点击
	if code, _ = clickAs(t, newToken); code != 200 {
		t.Fatalf("new token should click ok, got %d", code)
	}
	if taskStatus(t, taskID) != "opened" {
		t.Fatalf("task should be opened after new-token click")
	}
}

// TestClickTokenCannotLogin click Token 不得当登录 Token 使用（AuthRequired 拒绝 typ 非空）
func TestClickTokenCannotLogin(t *testing.T) {
	raw, _ := SignClickToken("FT-T1", "p-uuid")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/tasks", nil)
	c.Request.Header.Set("Authorization", "Bearer "+raw)
	middleware.AuthRequired()(c)

	if w.Code != 401 {
		t.Fatalf("click token must not pass login auth, got %d", w.Code)
	}
}

// TestLazyDegrade 降级链：>24h 未点击标记可二次提醒，>72h 惰性置 lost
func TestLazyDegrade(t *testing.T) {
	const taskID = "FT-HIS000002-2026Q4"
	resetTask(t, taskID)

	// 下发后把 sent_at 拨到 25h 前：列表应给 can_remind=true，状态仍 sms_sent
	if code, body := dispatchAs(t, taskID, nil); code != 200 {
		t.Fatalf("dispatch: %d %v", code, body)
	}
	if _, err := pool.Exec(context.Background(),
		`UPDATE followup_tasks SET sent_at = now() - interval '25 hours' WHERE task_id=$1`, taskID); err != nil {
		t.Fatalf("backdate sent_at: %v", err)
	}
	assertList(t, taskID, "sms_sent", true)

	// 拨到 73h 前：列表查询顺带惰性置 lost（终态）
	if _, err := pool.Exec(context.Background(),
		`UPDATE followup_tasks SET sent_at = now() - interval '73 hours' WHERE task_id=$1`, taskID); err != nil {
		t.Fatalf("backdate sent_at: %v", err)
	}
	assertList(t, taskID, "lost", false)

	// 终态不可下发
	code, _ := dispatchAs(t, taskID, nil)
	if code != 409 {
		t.Fatalf("lost task should be 409 not sendable, got %d", code)
	}
}

func assertList(t *testing.T, taskID, wantStatus string, wantRemind bool) {
	t.Helper()
	code, body := invoke(t, task.List(pool), "GET", "/api/v1/tasks", nil)
	if code != 200 {
		t.Fatalf("list: %d %v", code, body)
	}
	d := dataMap(body)
	tasks, _ := d["tasks"].([]any)
	for _, raw := range tasks {
		it, _ := raw.(map[string]any)
		if it["task_id"] == taskID {
			if it["status"] != wantStatus || it["can_remind"] != wantRemind {
				t.Fatalf("expect status=%s can_remind=%v, got %v %v", wantStatus, wantRemind, it["status"], it["can_remind"])
			}
			return
		}
	}
	t.Fatalf("task %s not found in list", taskID)
}
