package db

import (
	"context"
	"errors"
	"testing"

	ented "entgo.io/ent"
	"github.com/datey/datey/ent"
)

func TestTraceDBQuery_Success(t *testing.T) {
	ctx := context.Background()
	want := "hello"

	got, err := TraceDBQuery(ctx, "TestOp", func(ctx context.Context) (string, error) {
		return want, nil
	})
	if err != nil {
		t.Fatalf("TraceDBQuery: unexpected error: %v", err)
	}
	if got != want {
		t.Errorf("TraceDBQuery = %q, want %q", got, want)
	}
}

func TestTraceDBQuery_Error(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("db error")

	_, err := TraceDBQuery(ctx, "TestOpError", func(ctx context.Context) (string, error) {
		return "", sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("TraceDBQuery error = %v, want %v", err, sentinel)
	}
}

func TestTraceDBQuery_IntResult(t *testing.T) {
	ctx := context.Background()

	got, err := TraceDBQuery(ctx, "Count", func(ctx context.Context) (int, error) {
		return 42, nil
	})
	if err != nil {
		t.Fatalf("TraceDBQuery: unexpected error: %v", err)
	}
	if got != 42 {
		t.Errorf("TraceDBQuery = %d, want %d", got, 42)
	}
}

func TestTraceDBQuery_ContextPropagation(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("test"), "value")

	got, err := TraceDBQuery(ctx, "ContextTest", func(ctx context.Context) (string, error) {
		// Verify the context passed to dbFunc contains the value.
		v, _ := ctx.Value(contextKey("test")).(string)
		return v, nil
	})
	if err != nil {
		t.Fatalf("TraceDBQuery: unexpected error: %v", err)
	}
	if got != "value" {
		t.Errorf("context value = %q, want %q", got, "value")
	}
}

// contextKey avoids collisions with other context keys in tests.
type contextKey string

func TestEntQueryInterceptor_PassesThrough(t *testing.T) {
	called := false
	next := ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
		called = true
		return "ok", nil
	})

	got, err := EntQueryInterceptor(next).Query(context.Background(), nil)
	if err != nil {
		t.Fatalf("intercepted query: unexpected error: %v", err)
	}
	if !called {
		t.Error("expected next querier to be called")
	}
	if got != "ok" {
		t.Errorf("query result = %v, want %q", got, "ok")
	}
}

func TestEntQueryInterceptor_ReadsQueryContext(t *testing.T) {
	ctx := ented.NewQueryContext(context.Background(), &ent.QueryContext{Type: "Person", Op: "Query"})
	next := ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
		return 1, nil
	})

	got, err := EntQueryInterceptor(next).Query(ctx, nil)
	if err != nil {
		t.Fatalf("intercepted query: unexpected error: %v", err)
	}
	if got != 1 {
		t.Errorf("query result = %v, want 1", got)
	}
}
