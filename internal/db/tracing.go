package db

import (
	"context"
	"fmt"

	ented "entgo.io/ent"
	"github.com/datey/datey/ent"
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

// EntQueryInterceptor instruments every ent query with an OTel span by reusing
// TraceDBQuery. Register it with client.Intercept in Init. The operation name
// is derived from the ent.QueryContext that ent's generated code attaches to
// the context (e.g. "Person.Query" / "Query"). Note ent interceptors only wrap
// queries, not mutations.
func EntQueryInterceptor(next ent.Querier) ent.Querier {
	return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
		op := "query"
		if qc := ented.QueryFromContext(ctx); qc != nil {
			switch {
			case qc.Type != "" && qc.Op != "":
				op = qc.Type + "." + qc.Op
			case qc.Op != "":
				op = qc.Op
			}
		}
		return TraceDBQuery(ctx, op, func(ctx context.Context) (ent.Value, error) {
			return next.Query(ctx, q)
		})
	})
}
