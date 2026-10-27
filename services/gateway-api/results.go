package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func (a *App) handleHealerResult(parent context.Context, delivery amqp.Delivery) {
	ctx, span := otel.Tracer("sentinel/gateway/messaging").Start(extractRabbitTrace(parent, delivery.Headers), "consume healer.result", trace.WithSpanKind(trace.SpanKindConsumer), trace.WithAttributes(attribute.String("messaging.system", "rabbitmq"), attribute.String("messaging.destination.name", "healer.result")))
	defer span.End()
	var event map[string]any
	if e := json.Unmarshal(delivery.Body, &event); e != nil {
		span.RecordError(e)
		span.SetStatus(codes.Error, "Invalid healer result JSON")
		delivery.Reject(false)
		return
	}
	pipeline, _ := event["pipeline_id"].(string)
	if !validID(pipeline) {
		span.SetStatus(codes.Error, "Invalid healer result pipeline")
		delivery.Reject(false)
		return
	}
	span.SetAttributes(attribute.String("pipeline.id", pipeline))
	entry := map[string]any{"pipeline_id": pipeline, "stream": "stdout", "content": fmt.Sprintf("Healer verification: %v", event["verification_status"]), "timestamp": time.Now().UTC(), "event": "healer.result", "patch": event, "trace_id": span.SpanContext().TraceID().String()}
	b, _ := json.Marshal(entry)
	if e := a.redis.Publish(ctx, "pipeline:"+pipeline+":logs", b).Err(); e != nil {
		span.RecordError(e)
		span.SetStatus(codes.Error, "Redis result notification failed")
		delivery.Nack(false, true)
		return
	}
	if e := delivery.Ack(false); e != nil {
		span.RecordError(e)
		span.SetStatus(codes.Error, "RabbitMQ result acknowledgement failed")
	}
}
