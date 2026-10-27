package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var tracePropagation = propagation.TraceContext{}

func setupTracing(ctx context.Context) (func(context.Context) error, error) {
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", env("OTEL_SERVICE_NAME", "sentinel-gateway")))), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample()))}
	if env("OTEL_EXPORTER_OTLP_ENDPOINT", "") != "" || env("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "") != "" {
		exporter, e := otlptracehttp.New(ctx)
		if e != nil {
			return nil, e
		}
		options = append(options, sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(time.Second)))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(tracePropagation)
	return provider.Shutdown, nil
}
func traceHeaders(ctx context.Context) map[string]string {
	headers := map[string]string{}
	tracePropagation.Inject(ctx, propagation.MapCarrier(headers))
	return headers
}
func traceHeadersJSON(ctx context.Context) []byte { b, _ := json.Marshal(traceHeaders(ctx)); return b }
func restoreTrace(ctx context.Context, headers []byte) context.Context {
	carrier := map[string]string{}
	if json.Unmarshal(headers, &carrier) != nil {
		return ctx
	}
	return tracePropagation.Extract(ctx, propagation.MapCarrier(carrier))
}
func rabbitTraceHeaders(ctx context.Context) amqp.Table {
	headers := amqp.Table{}
	for key, value := range traceHeaders(ctx) {
		headers[key] = value
	}
	return headers
}
func extractRabbitTrace(ctx context.Context, headers amqp.Table) context.Context {
	carrier := map[string]string{}
	for _, key := range []string{"traceparent", "tracestate"} {
		switch value := headers[key].(type) {
		case string:
			carrier[key] = value
		case []byte:
			carrier[key] = string(value)
		}
	}
	return tracePropagation.Extract(ctx, propagation.MapCarrier(carrier))
}
func normalizedPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if validID(p) {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

type tracingWriter struct {
	http.ResponseWriter
	status int
}

func (w *tracingWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *tracingWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (w *tracingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *tracingWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
func (w *tracingWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response does not support connection hijacking")
	}
	conn, rw, e := h.Hijack()
	if e == nil {
		w.status = http.StatusSwitchingProtocols
	}
	return conn, rw, e
}
func traceHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parent := tracePropagation.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.Tracer("sentinel/gateway/http").Start(parent, r.Method+" "+normalizedPath(r.URL.Path), trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("url.path", normalizedPath(r.URL.Path))))
		defer span.End()
		wrapped := &tracingWriter{ResponseWriter: w}
		if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
			w.Header().Set("X-Trace-ID", sc.TraceID().String())
		}
		next.ServeHTTP(wrapped, r.WithContext(ctx))
		status := wrapped.status
		if status == 0 {
			status = 200
		}
		span.SetAttributes(attribute.Int("http.response.status_code", status))
		if status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
	})
}

type traceTransport struct{ base http.RoundTripper }

func (t traceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx, span := otel.Tracer("sentinel/gateway/client").Start(r.Context(), "HTTP "+r.Method, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attribute.String("http.request.method", r.Method), attribute.String("server.address", r.URL.Hostname())))
	defer span.End()
	request := r.Clone(ctx)
	request.Header = r.Header.Clone()
	tracePropagation.Inject(ctx, propagation.HeaderCarrier(request.Header))
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, e := base.RoundTrip(request)
	if e != nil {
		span.RecordError(e)
		span.SetStatus(codes.Error, "HTTP transport failed")
		return nil, e
	}
	span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
	if response.StatusCode >= 400 {
		span.SetStatus(codes.Error, http.StatusText(response.StatusCode))
	}
	return response, nil
}
