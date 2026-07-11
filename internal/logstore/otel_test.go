package logstore

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ── Helpers ────────────────────────────────────────────────────────────────

func setEnv(t *testing.T, key, value string) func() {
	t.Helper()
	old, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setenv %s=%s: %v", key, value, err)
	}
	return func() {
		if !had {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, old)
		}
	}
}

func unsetEnv(t *testing.T, key string) func() {
	t.Helper()
	old, had := os.LookupEnv(key)
	os.Unsetenv(key)
	return func() {
		if had {
			os.Setenv(key, old)
		}
	}
}

// ── InitTelemetry ──────────────────────────────────────────────────────────

func TestInitTelemetry_NoEndpoint(t *testing.T) {
	restore := unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")
	defer restore()
	restore2 := unsetEnv(t, "OTEL_ENDPOINT")
	defer restore2()

	telemetry, err := InitTelemetry(context.Background())
	if err != nil {
		t.Fatalf("InitTelemetry with no endpoint: unexpected error: %v", err)
	}
	if telemetry != nil {
		t.Fatal("InitTelemetry with no endpoint: expected nil Telemetry (noop), got non-nil")
	}
}

func TestInitTelemetry_OTEL_ENDPOINT_Fallback(t *testing.T) {
	restore := unsetEnv(t, "OTEL_EXPORTER_OTLP_ENDPOINT")
	defer restore()
	restore2 := setEnv(t, "OTEL_ENDPOINT", "http://localhost:99999")
	defer restore2()

	telemetry, err := InitTelemetry(context.Background())
	if err != nil {
		t.Fatalf("InitTelemetry with legacy OTEL_ENDPOINT: unexpected error: %v", err)
	}
	if telemetry == nil {
		t.Fatal("InitTelemetry with OTEL_ENDPOINT: expected non-nil Telemetry")
	}
	defer telemetry.Shutdown(context.Background())

	if telemetry.meterProvider == nil {
		t.Error("expected non-nil meterProvider")
	}
	if telemetry.loggerProvider == nil {
		t.Error("expected non-nil loggerProvider")
	}
	if telemetry.meter == nil {
		t.Error("expected non-nil meter")
	}
	if telemetry.otelHandler == nil {
		t.Error("expected non-nil otelHandler")
	}
}

// ── serviceName ────────────────────────────────────────────────────────────

func TestServiceName_Default(t *testing.T) {
	restore := unsetEnv(t, "OTEL_SERVICE_NAME")
	defer restore()
	if got := serviceName(); got != "datey" {
		t.Errorf("serviceName() = %q, want %q", got, "datey")
	}
}

func TestServiceName_FromEnv(t *testing.T) {
	restore := setEnv(t, "OTEL_SERVICE_NAME", "my-app")
	defer restore()
	if got := serviceName(); got != "my-app" {
		t.Errorf("serviceName() = %q, want %q", got, "my-app")
	}
}

// ── otelProtocol ───────────────────────────────────────────────────────────

func TestOtelProtocol_Default(t *testing.T) {
	restore := unsetEnv(t, "OTEL_EXPORTER_OTLP_PROTOCOL")
	defer restore()
	if got := otelProtocol(); got != "grpc" {
		t.Errorf("otelProtocol() = %q, want %q", got, "grpc")
	}
}

func TestOtelProtocol_Explicit(t *testing.T) {
	restore := setEnv(t, "OTEL_EXPORTER_OTLP_PROTOCOL", "http/protobuf")
	defer restore()
	if got := otelProtocol(); got != "http/protobuf" {
		t.Errorf("otelProtocol() = %q, want %q", got, "http/protobuf")
	}
}

// ── parseSamplerArg ────────────────────────────────────────────────────────

func TestParseSamplerArg(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want float64
	}{
		{"empty", "", 1.0},
		{"invalid", "abc", 1.0},
		{"zero", "0", 0.0},
		{"half", "0.5", 0.5},
		{"one", "1", 1.0},
		{"over max", "2.0", 1.0},
		{"under min", "-1", 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSamplerArg(tt.arg)
			if got != tt.want {
				t.Errorf("parseSamplerArg(%q) = %v, want %v", tt.arg, got, tt.want)
			}
		})
	}
}

// ── samplerFromEnv ─────────────────────────────────────────────────────────

func TestSamplerFromEnv(t *testing.T) {
	tests := []struct {
		name   string
		sampler string
		arg    string
	}{
		{"default empty", "", ""},
		{"parentbased_always_on", "parentbased_always_on", ""},
		{"always_on", "always_on", ""},
		{"parentbased_always_off", "parentbased_always_off", ""},
		{"always_off", "always_off", ""},
		{"parentbased_traceidratio", "parentbased_traceidratio", "0.5"},
		{"traceidratio", "traceidratio", "0.25"},
		{"unknown falls back", "unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restore1 := setEnv(t, "OTEL_TRACES_SAMPLER", tt.sampler)
			defer restore1()
			restore2 := setEnv(t, "OTEL_TRACES_SAMPLER_ARG", tt.arg)
			defer restore2()

			s := samplerFromEnv()
			if s == nil {
				t.Fatal("samplerFromEnv() returned nil")
			}
			// Should always return a non-nil sampler (no panics).
			_ = s.ShouldSample(sdktrace.SamplingParameters{})
		})
	}
}

// ── createHTTPMetrics ──────────────────────────────────────────────────────

func TestCreateHTTPMetrics(t *testing.T) {
	// Use a real MeterProvider to create a meter.
	mp := sdkmetric.NewMeterProvider()
	meter := mp.Meter("test")
	defer mp.Shutdown(context.Background())

	m, err := createHTTPMetrics(meter)
	if err != nil {
		t.Fatalf("createHTTPMetrics: unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("createHTTPMetrics: expected non-nil HTTPMetrics")
	}
	if m.RequestCount == nil {
		t.Error("expected non-nil RequestCount counter")
	}
	if m.RequestDuration == nil {
		t.Error("expected non-nil RequestDuration histogram")
	}
}

// ── RecordHTTPRequest ──────────────────────────────────────────────────────

func TestRecordHTTPRequest_Nil(t *testing.T) {
	// Should not panic.
	var m *HTTPMetrics
	m.RecordHTTPRequest(context.Background(), "GET", "/test", 200, time.Second)
}

func TestRecordHTTPRequest_Records(t *testing.T) {
	// Use a manual reader to verify metric values.
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	meter := mp.Meter("test")

	m, err := createHTTPMetrics(meter)
	if err != nil {
		t.Fatalf("createHTTPMetrics: %v", err)
	}

	ctx := context.Background()
	m.RecordHTTPRequest(ctx, "GET", "/test", 200, 500*time.Millisecond)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// We should have at least one scope metric with at least 2 metrics.
	if len(rm.ScopeMetrics) == 0 {
		t.Fatal("expected at least one ScopeMetrics")
	}
	// At minimum verify we don't panic and metrics flow through.
	_ = rm.ScopeMetrics[0].Metrics
}

// ── Telemetry Emit ─────────────────────────────────────────────────────────

func TestTelemetry_Emit_Nil(t *testing.T) {
	// Should not panic on nil receiver.
	var tlm *Telemetry
	tlm.Emit(context.Background(), testingLogRecord("test"))
}

func TestTelemetry_Emit_NilHandler(t *testing.T) {
	tlm := &Telemetry{otelHandler: nil}
	tlm.Emit(context.Background(), testingLogRecord("test"))
}

// ── Telemetry Shutdown ─────────────────────────────────────────────────────

func TestTelemetry_Shutdown_Nil(t *testing.T) {
	var tlm *Telemetry
	if err := tlm.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown on nil: unexpected error: %v", err)
	}
}

// ── helpers for tests ──────────────────────────────────────────────────────

func testingLogRecord(msg string) slog.Record {
	// slog.Record constructor: slog.NewRecord(time, level, msg, PC)
	// Use a simple approach:
	return slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
}
