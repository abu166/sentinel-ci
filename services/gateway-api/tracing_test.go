package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/pashagolub/pgxmock/v4"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const parentTrace = "00-1234567890abcdef1234567890abcdef-0123456789abcdef-01"

func recordTracing(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	old := otel.GetTracerProvider()
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { provider.Shutdown(context.Background()); otel.SetTracerProvider(old) })
	return exporter
}
func TestHTTPTracePreservesW3CParentAndRecordsStatus(t *testing.T) {
	exporter := recordTracing(t)
	id := uuid()
	r := httptest.NewRequest("GET", "/api/v1/pipelines/"+id+"?token=secret", nil)
	r.Header.Set("traceparent", parentTrace)
	r.Header.Set("tracestate", "sentinel=parent")
	r.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	traceHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := trace.SpanContextFromContext(r.Context())
		if sc.TraceID().String() != "1234567890abcdef1234567890abcdef" || sc.TraceState().String() != "sentinel=parent" {
			t.Fatal(sc)
		}
		w.WriteHeader(503)
	})).ServeHTTP(w, r)
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Parent.SpanID().String() != "0123456789abcdef" || spans[0].Status.Code != codes.Error || spans[0].Name != "GET /api/v1/pipelines/{id}" {
		t.Fatalf("%+v", spans)
	}
	if w.Header().Get("X-Trace-ID") != "1234567890abcdef1234567890abcdef" {
		t.Fatal(w.Header())
	}
	for _, attr := range spans[0].Attributes {
		if strings.Contains(attr.Value.AsString(), "secret") {
			t.Fatal("secret leaked in trace", attr)
		}
	}
}
func TestDurableTraceCarrierRoundTrip(t *testing.T) {
	recordTracing(t)
	ctx := tracePropagation.Extract(context.Background(), propagation.MapCarrier{"traceparent": parentTrace, "tracestate": "sentinel=durable", "baggage": "password=hidden"})
	encoded := traceHeadersJSON(ctx)
	var headers map[string]string
	if json.Unmarshal(encoded, &headers) != nil || len(headers) != 2 || headers["baggage"] != "" {
		t.Fatal(string(encoded))
	}
	restored := restoreTrace(context.Background(), encoded)
	if trace.SpanContextFromContext(restored).TraceID().String() != "1234567890abcdef1234567890abcdef" {
		t.Fatal("trace lost")
	}
	if trace.SpanContextFromContext(restoreTrace(context.Background(), []byte(`{`))).IsValid() {
		t.Fatal("invalid carrier created trace")
	}
}
func TestRabbitTraceCarrierHandlesAMQPWireTypes(t *testing.T) {
	ctx := extractRabbitTrace(context.Background(), amqp.Table{"traceparent": []byte(parentTrace), "tracestate": "sentinel=rabbit", "Authorization": "secret"})
	headers := rabbitTraceHeaders(ctx)
	if headers["traceparent"] != parentTrace || headers["tracestate"] != "sentinel=rabbit" || len(headers) != 2 {
		t.Fatal(headers)
	}
}
func TestOutboxRelayRestoresDurableTraceContext(t *testing.T) {
	exporter := recordTracing(t)
	a, db := mockApp(t)
	id := uuid()
	headers := []byte(`{"traceparent":"` + parentTrace + `","tracestate":"sentinel=outbox"}`)
	a.publishFunc = func(ctx context.Context, key string, v any) error {
		sc := trace.SpanContextFromContext(ctx)
		if sc.TraceID().String() != "1234567890abcdef1234567890abcdef" || sc.TraceState().String() != "sentinel=outbox" {
			t.Fatal("outbox propagation lost", sc)
		}
		return nil
	}
	db.ExpectBegin()
	db.ExpectQuery("SELECT id::text,event_type,payload,attempts,headers").WillReturnRows(pgxmock.NewRows([]string{"id", "kind", "payload", "attempts", "headers"}).AddRow(id, "jobs.dispatch", []byte(`{}`), 0, headers))
	db.ExpectExec("UPDATE outbox_events SET status='PUBLISHED'").WithArgs(id, 1).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	db.ExpectCommit()
	if worked, e := a.relayOne(context.Background()); e != nil || !worked {
		t.Fatal(worked, e)
	}
	spans := exporter.GetSpans()
	if len(spans) != 2 || spans[0].SpanKind != trace.SpanKindProducer || spans[1].Name != "outbox.relay jobs.dispatch" || spans[1].Parent.SpanID().String() != "0123456789abcdef" {
		t.Fatalf("%+v", spans)
	}
}
func TestInternalHTTPClientInjectsChildSpan(t *testing.T) {
	exporter := recordTracing(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc := trace.SpanContextFromContext(tracePropagation.Extract(r.Context(), propagation.HeaderCarrier(r.Header)))
		if sc.TraceID().String() != "1234567890abcdef1234567890abcdef" || sc.SpanID().String() == "0123456789abcdef" {
			t.Error("missing HTTP client child context", r.Header)
		}
		w.WriteHeader(204)
	}))
	defer server.Close()
	ctx := tracePropagation.Extract(context.Background(), propagation.MapCarrier{"traceparent": parentTrace})
	r, _ := http.NewRequestWithContext(ctx, "POST", server.URL, nil)
	response, e := (&http.Client{Transport: traceTransport{}, Timeout: time.Second}).Do(r)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].SpanKind != trace.SpanKindClient {
		t.Fatal(spans)
	}
}

type failRoundTripper struct{}

func (failRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("network failed")
}
func TestHTTPClientSpanRecordsTransportError(t *testing.T) {
	exporter := recordTracing(t)
	r, _ := http.NewRequest("GET", "http://publisher.internal", nil)
	if _, e := (traceTransport{base: failRoundTripper{}}).RoundTrip(r); e == nil {
		t.Fatal("error ignored")
	}
	spans := exporter.GetSpans()
	if len(spans) != 1 || spans[0].Status.Code != codes.Error {
		t.Fatal(spans)
	}
}

type deliveryAcknowledger struct{ acked, rejected, requeued bool }

func (a *deliveryAcknowledger) Ack(uint64, bool) error { a.acked = true; return nil }
func (a *deliveryAcknowledger) Nack(_ uint64, _ bool, requeue bool) error {
	a.rejected = true
	a.requeued = requeue
	return nil
}
func (a *deliveryAcknowledger) Reject(_ uint64, requeue bool) error {
	a.rejected = true
	a.requeued = requeue
	return nil
}
func TestResultConsumerContinuesIncomingTrace(t *testing.T) {
	exporter := recordTracing(t)
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	a := &App{redis: rc}
	ack := &deliveryAcknowledger{}
	pipeline := uuid()
	body, _ := json.Marshal(map[string]any{"pipeline_id": pipeline, "verification_status": "VERIFIED"})
	a.handleHealerResult(context.Background(), amqp.Delivery{Acknowledger: ack, Body: body, Headers: amqp.Table{"traceparent": parentTrace}})
	spans := exporter.GetSpans()
	if !ack.acked || ack.rejected || len(spans) != 1 || spans[0].SpanKind != trace.SpanKindConsumer || spans[0].Parent.SpanID().String() != "0123456789abcdef" {
		t.Fatal(ack, spans)
	}
}
func TestResultConsumerRejectsMalformedInputWithSpanError(t *testing.T) {
	exporter := recordTracing(t)
	a := &App{}
	for _, body := range []string{"{", `{"pipeline_id":"bad"}`} {
		ack := &deliveryAcknowledger{}
		a.handleHealerResult(context.Background(), amqp.Delivery{Acknowledger: ack, Body: []byte(body), Headers: amqp.Table{"traceparent": parentTrace}})
		if !ack.rejected || ack.requeued || ack.acked {
			t.Fatal(ack)
		}
	}
	spans := exporter.GetSpans()
	if len(spans) != 2 || spans[0].Status.Code != codes.Error || spans[1].Status.Code != codes.Error {
		t.Fatal(spans)
	}
}
