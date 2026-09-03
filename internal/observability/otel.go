package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Init wires a stdout trace exporter for local dev (Phase 0 scaffolding).
// A collector/OTLP exporter replaces this in later phases; the tracer
// provider + service.name resource contract stays the same.
func Init(ctx context.Context, service string) (func(context.Context) error, error) {
	exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, fmt.Errorf("otel: stdout exporter: %w", err)
	}
	res, err := resource.Merge(resource.Default(),
		resource.NewWithAttributes("", attribute.String("service.name", service)))
	if err != nil {
		return nil, fmt.Errorf("otel: resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// DummySpan emits one span so `tide doctor` / smoke tests can prove the
// pipeline is wired. T001/T006 acceptance: span appears in output.
func DummySpan(ctx context.Context) {
	tr := otel.Tracer("tide/phase0")
	ctx, span := tr.Start(ctx, "tide.phase0.dummy")
	defer span.End()
	_ = ctx
}
