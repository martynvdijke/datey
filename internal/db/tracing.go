package db

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/datey/datey/internal/db"

// TraceDBQuery wraps a database operation with an OTel span for observability.
// It creates a span named "db.query.<operation>", records any error on the span,
// and returns both the result and any error from dbFunc.
//
// Usage:
//
//	rows, err := TraceDBQuery(ctx, "ListPeople", func(ctx context.Context) (any, error) {
//	    return client.Person.Query().All(ctx)
//	})
func TraceDBQuery[T any](ctx context.Context, operation string, dbFunc func(context.Context) (T, error)) (T, error) {
	tracer := otel.Tracer(tracerName)
	ctx, span := tracer.Start(ctx,
		fmt.Sprintf("db.query.%s", operation),
		trace.WithAttributes(
			attribute.String("db.operation", operation),
			attribute.String("db.system", "sqlite"),
		),
	)
	defer span.End()

	result, err := dbFunc(ctx)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
	return result, err
}
