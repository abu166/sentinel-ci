"""OpenTelemetry spans with W3C-only cross-service propagation."""
import os
from opentelemetry import trace
from opentelemetry.trace import SpanKind, Status, StatusCode
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter

propagator = TraceContextTextMapPropagator()
tracer = trace.get_tracer("sentinel.healer")
_provider = None


def configure():
    global _provider
    if _provider is None:
        _provider = TracerProvider(resource=Resource.create({"service.name": os.getenv("OTEL_SERVICE_NAME", "sentinel-healer")}))
        endpoint = os.getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
        if not endpoint and os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT"):
            endpoint = os.environ["OTEL_EXPORTER_OTLP_ENDPOINT"].rstrip("/") + "/v1/traces"
        if endpoint:
            _provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=endpoint, timeout=5)))
        trace.set_tracer_provider(_provider)
    return _provider


def headers() -> dict[str, str]:
    carrier = {}
    propagator.inject(carrier)
    return {key: value for key, value in carrier.items() if key in {"traceparent", "tracestate"}}


def parent(carrier):
    safe = {str(key).lower(): value.decode(errors="replace") if isinstance(value, bytes) else str(value) for key, value in (carrier or {}).items() if str(key).lower() in {"traceparent", "tracestate"}}
    return propagator.extract(safe)


def flush():
    if _provider is not None:
        _provider.force_flush(timeout_millis=5000)


async def middleware(request, call_next):
    with tracer.start_as_current_span(f"HTTP {request.method} {request.url.path}", context=parent(request.headers), kind=SpanKind.SERVER,
                                      attributes={"http.request.method": request.method, "url.path": request.url.path}) as span:
        response = await call_next(request)
        span.set_attribute("http.response.status_code", response.status_code)
        if response.status_code >= 500:
            span.set_status(Status(StatusCode.ERROR))
        for key, value in headers().items():
            response.headers[key] = value
        return response
