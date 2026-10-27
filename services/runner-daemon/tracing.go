package main

import (
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

var tracePropagation = propagation.TraceContext{}

func setupTracing(ctx context.Context) (func(context.Context) error, error) {
	options := []sdktrace.TracerProviderOption{sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", env("OTEL_SERVICE_NAME", "sentinel-runner")))), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample()))}
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
