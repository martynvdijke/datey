package db

import (
	"context"
	"errors"
	"testing"

	ented "entgo.io/ent"
	"github.com/datey/datey/ent"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// useSpanRecorder swaps the global tracer provider for one backed by a span
// recorder, restoring the previous provider when the test ends.
func useSpanRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
		_ = tp.Shutdown(context.Background())
	})
	return rec
}

func spanAttributes(span sdktrace.ReadOnlySpan) map[string]string {
	attrs := make(map[string]string, len(span.Attributes()))
	for _, kv := range span.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	return attrs
}

func TestTraceDBQuery_EmitsSpan(t *testing.T) {
	rec := useSpanRecorder(t)

	_, err := TraceDBQuery(context.Background(), "Person.Query", func(context.Context) (int, error) {
		return 1, nil
	})
	if err != nil {
		t.Fatalf("TraceDBQuery: %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if want := "db.query.Person.Query"; span.Name() != want {
		t.Errorf("span name = %q, want %q", span.Name(), want)
	}
	attrs := spanAttributes(span)
	if got := attrs["db.operation"]; got != "Person.Query" {
		t.Errorf("db.operation = %q, want %q", got, "Person.Query")
	}
	if got := attrs["db.system"]; got != "sqlite" {
		t.Errorf("db.system = %q, want %q", got, "sqlite")
	}
	if span.Status().Code != codes.Unset {
		t.Errorf("span status = %v, want Unset for success", span.Status().Code)
	}
}

func TestTraceDBQuery_RecordsError(t *testing.T) {
	rec := useSpanRecorder(t)
	sentinel := errors.New("boom")

	if _, err := TraceDBQuery(context.Background(), "Person.Create", func(context.Context) (int, error) {
		return 0, sentinel
	}); !errors.Is(err, sentinel) {
		t.Fatalf("TraceDBQuery error = %v, want %v", err, sentinel)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	span := spans[0]
	if span.Status().Code != codes.Error {
		t.Errorf("span status = %v, want Error", span.Status().Code)
	}
	if len(span.Events()) == 0 {
		t.Error("expected a recorded error event on the span")
	}
}

func TestEntQueryInterceptor_EmitsTypedSpan(t *testing.T) {
	rec := useSpanRecorder(t)
	ctx := ented.NewQueryContext(context.Background(), &ent.QueryContext{Type: "Person", Op: "Query"})
	next := ent.QuerierFunc(func(context.Context, ent.Query) (ent.Value, error) {
		return 1, nil
	})

	if _, err := EntQueryInterceptor(next).Query(ctx, nil); err != nil {
		t.Fatalf("intercepted query: %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if want := "db.query.Person.Query"; spans[0].Name() != want {
		t.Errorf("span name = %q, want %q", spans[0].Name(), want)
	}
}
