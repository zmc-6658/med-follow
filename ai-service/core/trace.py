"""基于 OpenTelemetry SDK 的手工埋点包装层（PRD 5.7，V1.2.3）。

与 gateway/internal/trace 同构：业务代码只依赖本模块的常量与 emit_event/hash_id/span，
OTel 细节全部收口在此。PII 防线保留在包装层：OTel 不认识业务 PII，
name/phone/id_card 键在输出前过滤，标识一律以 hash_id 的 16 位哈希出现。

用法：
    init("medfollow-ai-service")            # 进程启动时一次
    with span("ai.followup_turn") as sp:    # 手工埋点 span
        emit_event(EVENT_TURN_COMPLETED, rounds=3, cost_ms=120)
"""

import hashlib
import json
import threading
from contextlib import contextmanager
from datetime import datetime, timezone

from opentelemetry import trace as otel_trace
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor, SpanExporter, SpanExportResult

# PRD 第七章埋点事件常量。仅为尚不存在的业务流程保留常量，触发点随业务落地逐个接入
EVENT_SMS_SENT = "sms_sent"
EVENT_LINK_CLICKED = "link_clicked"
EVENT_FOLLOWUP_STARTED = "followup_started"
EVENT_TURN_COMPLETED = "turn_completed"
EVENT_FIELD_FILLED = "field_filled"
EVENT_FIELD_CORRECTED = "field_corrected"
EVENT_RULE_TRIGGERED = "rule_triggered"
EVENT_ALERT_CREATED = "alert_created"
EVENT_ALERT_HANDLED = "alert_handled"
EVENT_SUBMIT_SUCCESS = "submit_success"
EVENT_SYNC_FAILED = "sync_failed"
EVENT_FALLBACK_TO_TEXT = "fallback_to_text"

# P1 预留：LLM 调用相关 span 属性约定（token 统计接入时直接使用）
ATTR_PROMPT_TOKENS = "prompt_tokens"
ATTR_COMPLETION_TOKENS = "completion_tokens"
ATTR_MODEL = "model"
ATTR_COST = "cost"

# 业务 PII 键黑名单：埋点输出只允许 task_id/patient_id 的 hash_id 形式
_PII_KEYS = {"name", "phone", "id_card"}

_provider: TracerProvider | None = None
_lock = threading.Lock()


def hash_id(value: str) -> str:
    """标识脱敏：sha256 前 16 个十六进制字符，trace 中不出现明文 ID"""
    return hashlib.sha256(value.encode("utf-8")).hexdigest()[:16]


def init(service_name: str = "medfollow-ai-service",
         otlp_endpoint: str | None = None,
         output=None) -> None:
    """初始化全局 TracerProvider：BatchSpanProcessor + stdout JSON lines 导出（dev 用）。

    otlp_endpoint 为 P2 预留：届时填 LangFuse 的 OTLP 地址（LangFuse 原生消费 OTLP）；
    为空则仅输出 JSON lines 到本地（output 默认 sys.stdout）。
    """
    global _provider
    with _lock:
        if _provider is not None:
            return
        _provider = TracerProvider(resource=Resource.create({"service.name": service_name}))
        _provider.add_span_processor(BatchSpanProcessor(JsonLinesSpanExporter(output)))
        # P2：otlp_endpoint 非空时在此追加 OTLPSpanExporter（opentelemetry-exporter-otlp）
        otel_trace.set_tracer_provider(_provider)


def shutdown() -> None:
    """进程退出前调用，冲刷未导出的 span"""
    global _provider
    if _provider is not None:
        _provider.shutdown()
        _provider = None


def _tracer() -> otel_trace.Tracer:
    if _provider is None:
        raise RuntimeError("trace not initialized: call trace.init() first")
    return _provider.get_tracer(__name__)


@contextmanager
def span(name: str, **attrs):
    """手工埋点 span 上下文管理器，attrs 中的 PII 键会被过滤"""
    with _tracer().start_as_current_span(name, attributes=_kv(attrs)) as s:
        yield s


def emit_event(event: str, **attrs) -> None:
    """在当前 span 上记录一个 PRD 埋点事件，attrs 中的 PII 键会被过滤"""
    otel_trace.get_current_span().add_event(event, attributes=_kv(attrs))


class JsonLinesSpanExporter(SpanExporter):
    """每个 span 导出为一行 JSON（字段对齐 OTel 惯例），供 dev 阶段本地查看"""

    def __init__(self, output=None):
        from sys import stdout
        self._out = output if output is not None else stdout
        self._lock = threading.Lock()

    def export(self, spans) -> SpanExportResult:
        lines = [json.dumps(self._to_dict(s), ensure_ascii=False) for s in spans]
        with self._lock:
            self._out.write("\n".join(lines) + "\n")
            self._out.flush()
        return SpanExportResult.SUCCESS

    def shutdown(self) -> None:
        pass

    @staticmethod
    def _to_dict(s) -> dict:
        ctx = s.get_span_context()
        parent = s.parent
        return {
            "name": s.name,
            "trace_id": format(ctx.trace_id, "032x"),
            "span_id": format(ctx.span_id, "016x"),
            "parent_span_id": format(parent.span_id, "016x") if parent else None,
            "start_time": datetime.fromtimestamp(s.start_time / 1e9, tz=timezone.utc).isoformat(),
            "end_time": datetime.fromtimestamp(s.end_time / 1e9, tz=timezone.utc).isoformat(),
            "attributes": dict(s.attributes),
            "events": [
                {"name": e.name, "attributes": dict(e.attributes)} for e in s.events
            ],
            "status": s.status.status_code.name.lower(),
        }


def _kv(attrs: dict) -> dict:
    """过滤 PII 键后转换为 OTel 属性字典"""
    return {k: v for k, v in attrs.items() if k not in _PII_KEYS}
