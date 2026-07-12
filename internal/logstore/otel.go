package logstore

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"strconv"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Telemetry holds all OpenTelemetry providers and provides emit/shutdown.
type Telemetry struct {
	tracerProvider traceProvider
	meterProvider  *sdkmetric.MeterProvider
	loggerProvider *sdklog.LoggerProvider
	meter          metric.Meter

	// otelHandler is the otelslog bridge handler that routes slog records
	// through the OTel logs SDK with trace context correlation.
	otelHandler slog.Handler

	// HTTPMetrics are OTel instruments for HTTP request observability.
	HTTPMetrics *HTTPMetrics
}

// traceProvider is the interface we need for shutdown.
type traceProvider interface {
	Shutdown(ctx context.Context) error
}

// HTTPMetrics holds OTel instruments for HTTP request observability.
type HTTPMetrics struct {
	RequestCount    metric.Int64Counter
	RequestDuration metric.Float64Histogram
}

// serviceName returns the OTel service name from env or defaults to "datey".
func serviceName() string {
	if name := os.Getenv("OTEL_SERVICE_NAME"); name != "" {
		return name
	}
	return "datey"
}

// otelProtocol returns the OTLP protocol from env (default: "grpc").
func otelProtocol() string {
	p := os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	if p == "" {
		return "grpc"
	}
	return p
}

// InitTelemetry initialises tracer, meter, and logger providers with OTLP
// export. It reads standard OTel environment variables for configuration:
//   - OTEL_EXPORTER_OTLP_ENDPOINT (or OTEL_ENDPOINT as legacy fallback)
//   - OTEL_EXPORTER_OTLP_PROTOCOL ("grpc" or "http/protobuf", default "grpc")
//   - OTEL_TRACES_SAMPLER (always_on, always_off, traceidratio, parentbased_*)
//   - OTEL_TRACES_SAMPLER_ARG (float for traceidratio)
//   - OTEL_SERVICE_NAME (default "datey")
//   - OTEL_RESOURCE_ATTRIBUTES (comma-separated key=value pairs)
//
// If no OTLP endpoint is configured, it returns nil (noop).
func InitTelemetry(ctx context.Context) (*Telemetry, error) {
	// Determine the OTLP endpoint, preferring the new standard env var.
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_ENDPOINT")
	}
	if endpoint == "" {
		// No endpoint → noop (graceful degradation: no OTel without config).
		return nil, nil
	}

	// Ensure the standard env var is set for downstream OTel SDK auto-detection.
	// Setenv only fails on invalid key (e.g. contains null byte), which can't
	// happen with the string literal used here.
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		_ = os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", endpoint)
	}

	// ── Resource detection ──────────────────────────────────────────────
	res, err := resource.New(ctx,
		resource.WithFromEnv(), // reads OTEL_RESOURCE_ATTRIBUTES
		resource.WithAttributes(
			semconv.ServiceName(serviceName()),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTel resource: %w", err)
	}

	// ── Protocol selection ──────────────────────────────────────────────
	proto := otelProtocol()

	// ── Tracer provider ─────────────────────────────────────────────────
	traceExporter, err := newTraceExporter(ctx, proto)
	if err != nil {
		return nil, fmt.Errorf("create trace exporter: %w", err)
	}

	sampler := samplerFromEnv()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)

	// ── Meter provider ──────────────────────────────────────────────────
	metricExporter, err := newMetricExporter(ctx, proto)
	if err != nil {
		_ = tp.Shutdown(ctx)
		return nil, fmt.Errorf("create metric exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)),
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	// ── Logger provider ─────────────────────────────────────────────────
	logExporter, err := newLogExporter(ctx, proto)
	if err != nil {
		_ = tp.Shutdown(ctx)
		_ = mp.Shutdown(ctx)
		return nil, fmt.Errorf("create log exporter: %w", err)
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
		sdklog.WithResource(res),
	)

	// ── OTel slog bridge ───────────────────────────────────────────────
	// The otelslog bridge creates an slog.Handler that routes log records
	// through the OTel logs SDK. It automatically extracts trace context
	// (trace_id, span_id) from the context.Context, providing log-to-trace
	// correlation for logs emitted within a span.
	otelHandler := otelslog.NewHandler("datey",
		otelslog.WithLoggerProvider(lp),
	)

	meter := mp.Meter("datey")
	telemetry := &Telemetry{
		tracerProvider: tp,
		meterProvider:  mp,
		loggerProvider: lp,
		otelHandler:    otelHandler,
		meter:          meter,
	}

	// ── Create HTTP metric instruments ──────────────────────────────────
	httpMetrics, err := createHTTPMetrics(meter)
	if err != nil {
		_ = telemetry.Shutdown(ctx)
		return nil, fmt.Errorf("create HTTP metrics: %w", err)
	}
	telemetry.HTTPMetrics = httpMetrics

	return telemetry, nil
}

// Emit forwards an slog.Record to the OTel logs pipeline via the otelslog
// bridge. The bridge automatically extracts trace context (trace_id, span_id)
// from ctx, enabling log-to-trace correlation.
func (t *Telemetry) Emit(ctx context.Context, r slog.Record) {
	if t == nil || t.otelHandler == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("otel export panic recovered", "error", rec)
		}
	}()

	_ = t.otelHandler.Handle(ctx, r)
}

// Shutdown gracefully shuts down all OTel providers in reverse order.
// It flushes pending spans, metrics, and logs before returning.
func (t *Telemetry) Shutdown(ctx context.Context) error {
	if t == nil {
		return nil
	}
	// Shutdown in reverse dependency order: trace → metric → log.
	var errs []error

	if err := t.tracerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("tracer provider shutdown: %w", err))
	}
	if err := t.meterProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("meter provider shutdown: %w", err))
	}
	if err := t.loggerProvider.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("logger provider shutdown: %w", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf("otel shutdown errors: %v", errs)
	}
	return nil
}

// newTraceExporter creates a trace exporter based on the protocol.
func newTraceExporter(ctx context.Context, proto string) (sdktrace.SpanExporter, error) {
	switch proto {
	case "http/protobuf":
		return otlptracehttp.New(ctx)
	default:
		return otlptracegrpc.New(ctx)
	}
}

// newMetricExporter creates a metric exporter based on the protocol.
func newMetricExporter(ctx context.Context, proto string) (sdkmetric.Exporter, error) {
	switch proto {
	case "http/protobuf":
		return otlpmetrichttp.New(ctx)
	default:
		return otlpmetricgrpc.New(ctx)
	}
}

// newLogExporter creates a log exporter based on the protocol.
func newLogExporter(ctx context.Context, proto string) (sdklog.Exporter, error) {
	switch proto {
	case "http/protobuf":
		return otlploghttp.New(ctx)
	default:
		return otlploggrpc.New(ctx)
	}
}

// samplerFromEnv reads OTEL_TRACES_SAMPLER and OTEL_TRACES_SAMPLER_ARG
// env vars and returns the corresponding OTel SDK sampler.
func samplerFromEnv() sdktrace.Sampler {
	samplerName := os.Getenv("OTEL_TRACES_SAMPLER")
	arg := os.Getenv("OTEL_TRACES_SAMPLER_ARG")

	switch samplerName {
	case "", "parentbased_always_on":
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	case "always_on":
		return sdktrace.AlwaysSample()
	case "parentbased_always_off":
		return sdktrace.ParentBased(sdktrace.NeverSample())
	case "always_off":
		return sdktrace.NeverSample()
	case "parentbased_traceidratio":
		return sdktrace.ParentBased(sdktrace.TraceIDRatioBased(parseSamplerArg(arg)))
	case "traceidratio":
		return sdktrace.TraceIDRatioBased(parseSamplerArg(arg))
	default:
		slog.Warn("unknown OTEL_TRACES_SAMPLER, falling back to parentbased_always_on", "sampler", samplerName)
		return sdktrace.ParentBased(sdktrace.AlwaysSample())
	}
}

// parseSamplerArg parses the sampler argument as a float64.
// Returns 1.0 (100%) on invalid input.
func parseSamplerArg(arg string) float64 {
	if arg == "" {
		return 1.0
	}
	v, err := strconv.ParseFloat(arg, 64)
	if err != nil {
		slog.Warn("invalid OTEL_TRACES_SAMPLER_ARG, defaulting to 1.0", "arg", arg, "error", err)
		return 1.0
	}
	return math.Max(0, math.Min(1, v))
}

// createHTTPMetrics creates OTel instruments for HTTP request observability.
func createHTTPMetrics(meter metric.Meter) (*HTTPMetrics, error) {
	requestCount, err := meter.Int64Counter(
		"otel_http_requests_total",
		metric.WithDescription("Total number of HTTP requests"),
		metric.WithUnit("{count}"),
	)
	if err != nil {
		return nil, fmt.Errorf("create request count counter: %w", err)
	}

	requestDuration, err := meter.Float64Histogram(
		"otel_http_request_duration_seconds",
		metric.WithDescription("Duration of HTTP requests in seconds"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return nil, fmt.Errorf("create request duration histogram: %w", err)
	}

	return &HTTPMetrics{
		RequestCount:    requestCount,
		RequestDuration: requestDuration,
	}, nil
}

// RecordHTTPRequest records metrics for an HTTP request.
func (m *HTTPMetrics) RecordHTTPRequest(ctx context.Context, method, path string, statusCode int, duration time.Duration) {
	if m == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("http.method", method),
		attribute.String("http.path", path),
		attribute.Int("http.status_code", statusCode),
	}
	m.RequestCount.Add(ctx, 1, metric.WithAttributes(attrs...))
	m.RequestDuration.Record(ctx, duration.Seconds(), metric.WithAttributes(attrs...))
}
