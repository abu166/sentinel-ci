package main

import (
	"context"
	"encoding/json"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"testing"
)

func TestTraceSurvivesDurableOutboxAndRabbitWire(t *testing.T) {
	parent := "00-1234567890abcdef1234567890abcdef-0123456789abcdef-01"
	ctx := tracePropagation.Extract(context.Background(), propagation.MapCarrier{"traceparent": parent, "tracestate": "sentinel=runner", "baggage": "password=hidden"})
	encoded := traceHeadersJSON(ctx)
	var carrier map[string]string
	if err := json.Unmarshal(encoded, &carrier); err != nil || len(carrier) != 2 {
		t.Fatalf("invalid carrier: %s", encoded)
	}
	restored := restoreTrace(context.Background(), encoded)
	headers := rabbitTraceHeaders(restored)
	headers["traceparent"] = []byte(headers["traceparent"].(string))
	headers["authorization"] = "secret"
	received := extractRabbitTrace(context.Background(), headers)
	sc := trace.SpanContextFromContext(received)
	if sc.TraceID().String() != "1234567890abcdef1234567890abcdef" || sc.TraceState().String() != "sentinel=runner" || !sc.IsRemote() {
		t.Fatal(sc)
	}
	if len(rabbitTraceHeaders(received)) != 2 {
		t.Fatal("unapproved headers propagated")
	}
}
func TestInvalidTraceCarrierDoesNotCreateContext(t *testing.T) {
	for _, raw := range [][]byte{nil, []byte("{"), []byte(`{"traceparent":"bad"}`)} {
		if trace.SpanContextFromContext(restoreTrace(context.Background(), raw)).IsValid() {
			t.Fatal("invalid trace accepted")
		}
	}
	if trace.SpanContextFromContext(extractRabbitTrace(context.Background(), amqp.Table{"traceparent": 123})).IsValid() {
		t.Fatal("invalid AMQP type accepted")
	}
}
