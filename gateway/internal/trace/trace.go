package trace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/gin-gonic/gin"
)

// PRD 第七章埋点事件常量。仅为尚不存在的业务流程保留常量，触发点随业务落地逐个接入
const (
	EventSmsSent         = "sms_sent"
	EventLinkClicked     = "link_clicked"
	EventFollowupStarted = "followup_started"
	EventTurnCompleted   = "turn_completed"
	EventFieldFilled     = "field_filled"
	EventFieldCorrected  = "field_corrected"
	EventRuleTriggered   = "rule_triggered"
	EventAlertCreated    = "alert_created"
	EventAlertHandled    = "alert_handled"
	EventSubmitSuccess   = "submit_success"
	EventSyncFailed      = "sync_failed"
	EventFallbackToText  = "fallback_to_text"
)

// Span 名常量：进程内关键链路
const (
	SpanHTTP         = "http.request"
	SpanSyncRun      = "sync.run"
	SpanTaskGenerate = "task.generate"
	SpanMCPToolCall  = "mcp.tool_call"
)

// piiKeys 业务 PII 键黑名单：埋点输出只允许 task_id/patient_id 的 HashID 形式
var piiKeys = map[string]bool{"name": true, "phone": true, "id_card": true}

var (
	tracer oteltrace.Tracer
	output io.Writer = os.Stdout
)

// SetOutput 设置 trace 输出目标，必须在 Init 之前调用。
// MCP Server 进程的 stdout 是 JSON-RPC 协议通道，启动时须切到 os.Stderr。
func SetOutput(w io.Writer) { output = w }

// Config Init 配置。OtlpEndpoint 为 P2 预留：届时填 LangFuse 的 OTLP 地址，
// LangFuse 原生消费 OTLP、零适配；为空则仅输出 JSON lines 到本地
type Config struct {
	ServiceName  string
	OTLPEndpoint string
}

// Init 初始化全局 TracerProvider：BatchSpanProcessor + stdout JSON lines 导出器（dev 用）。
// 返回 shutdown，进程退出前调用以冲刷未导出的 span。
func Init(cfg Config) (shutdown func(ctx context.Context) error) {
	exp, err := stdouttrace.New(stdouttrace.WithWriter(output))
	if err != nil {
		panic(fmt.Sprintf("trace: new stdout exporter: %v", err))
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", cfg.ServiceName))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	tracer = tp.Tracer("MedFollow/internal/trace")

	// P2：cfg.OTLPEndpoint 非空时在此追加 OTLP exporter（otlptracehttp），输出地址指向 LangFuse
	return tp.Shutdown
}

// HashID 标识脱敏：sha256 前 16 个十六进制字符，trace 中不出现 task_id/patient_id 明文
func HashID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:8])
}

// StartSpan 在 ctx 上开启一个手工埋点 span，attrs 中的 PII 键会被过滤。
// 用法：ctx, sp := trace.StartSpan(ctx, trace.SpanSyncRun); defer sp.End()
func StartSpan(ctx context.Context, name string, attrs ...map[string]any) (context.Context, *Span) {
	var m map[string]any
	if len(attrs) > 0 {
		m = attrs[0]
	}
	ctx, s := tracer.Start(ctx, name, oteltrace.WithAttributes(toAttrs(m)...))
	return ctx, &Span{s: s}
}

// Span 对 OTel span 的薄包装，供调用点 End / 标记错误 / 补充属性
type Span struct {
	s oteltrace.Span
}

func (sp *Span) End() { sp.s.End() }

// SetError 记录执行失败（错误对象原文不含 PII 时方可传入）
func (sp *Span) SetError(err error) {
	if err != nil {
		sp.s.RecordError(err)
		sp.s.SetStatus(codes.Error, err.Error())
	}
}

func (sp *Span) SetAttrs(attrs map[string]any) { sp.s.SetAttributes(toAttrs(attrs)...) }

// EmitEvent 在 ctx 当前 span 上记录一个 PRD 埋点事件，attrs 中的 PII 键会被过滤
func EmitEvent(ctx context.Context, event string, attrs map[string]any) {
	oteltrace.SpanFromContext(ctx).AddEvent(event, oteltrace.WithAttributes(toAttrs(attrs)...))
}

// Middleware HTTP 链路埋点：解析 W3C traceparent（有则接续上游链路），请求结束回写 X-Trace-ID
func Middleware() gin.HandlerFunc {
	prop := propagation.TraceContext{}
	return func(c *gin.Context) {
		ctx := prop.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, sp := tracer.Start(ctx, SpanHTTP, oteltrace.WithAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.path", c.Request.URL.Path),
		))
		defer sp.End()

		c.Header("X-Trace-ID", sp.SpanContext().TraceID().String())
		c.Request = c.Request.WithContext(ctx)
		c.Next()

		sp.SetAttributes(attribute.Int("http.status_code", c.Writer.Status()))
	}
}

// toAttrs 过滤 PII 键后转换为 OTel 属性
func toAttrs(m map[string]any) []attribute.KeyValue {
	if len(m) == 0 {
		return nil
	}
	kv := make([]attribute.KeyValue, 0, len(m))
	for k, v := range m {
		if piiKeys[k] {
			continue
		}
		switch val := v.(type) {
		case string:
			kv = append(kv, attribute.String(k, val))
		case int:
			kv = append(kv, attribute.Int(k, val))
		case int64:
			kv = append(kv, attribute.Int64(k, val))
		case float64:
			kv = append(kv, attribute.Float64(k, val))
		case bool:
			kv = append(kv, attribute.Bool(k, val))
		case time.Duration:
			kv = append(kv, attribute.Int64(k+"_ms", val.Milliseconds()))
		default:
			kv = append(kv, attribute.String(k, fmt.Sprint(val)))
		}
	}
	return kv
}
