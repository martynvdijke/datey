package logstore

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// OTelHelper wraps an OTel logger provider and provides a method to
// emit slog records as OTel log records.
type OTelHelper struct {
	provider *log.LoggerProvider
	logger   otellog.Logger
}

// NewOTelHelper creates a new OTelHelper connected to the given OTLP endpoint.
func NewOTelHelper(endpoint string) (*OTelHelper, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	exporter, err := otlploggrpc.New(ctx,
		otlploggrpc.WithEndpoint(endpoint),
		otlploggrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTel log exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("datey"),
			semconv.ServiceVersion("1.0.2"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTel resource: %w", err)
	}

	provider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(exporter)),
		log.WithResource(res),
	)

	logger := provider.Logger("datey")

	return &OTelHelper{
		provider: provider,
		logger:   logger,
	}, nil
}

// Emit converts an slog.Record to an OTel log record and emits it.
func (h *OTelHelper) Emit(ctx context.Context, r slog.Record) {
	defer func() {
		if rec := recover(); rec != nil {
			slog.Warn("otel export panic recovered", "error", rec)
		}
	}()

	var record otellog.Record
	record.SetTimestamp(r.Time)
	record.SetSeverity(slogToOtelSeverity(r.Level))
	record.SetSeverityText(r.Level.String())
	record.SetBody(otellog.StringValue(r.Message))

	// Copy attributes from the slog record.
	r.Attrs(func(a slog.Attr) bool {
		record.AddAttributes(attrToOtel(a))
		return true
	})

	h.logger.Emit(ctx, record)
}

// slogToOtelSeverity maps slog.Level to OTel Severity.
func slogToOtelSeverity(l slog.Level) otellog.Severity {
	switch {
	case l < slog.LevelInfo:
		return otellog.SeverityDebug1
	case l < slog.LevelWarn:
		return otellog.SeverityInfo1
	case l < slog.LevelError:
		return otellog.SeverityWarn1
	default:
		return otellog.SeverityError1
	}
}

// attrToOtel converts a slog.Attr to an OTel KeyValue.
func attrToOtel(a slog.Attr) otellog.KeyValue {
	return otellog.String(a.Key, fmt.Sprintf("%v", a.Value.Any()))
}
