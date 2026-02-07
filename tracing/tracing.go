// Package tracing provides OpenTelemetry tracing for the Code to Arch MCP server.
package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

const TracerName = "code-to-arch-mcp"

// Config holds tracing configuration.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	Enabled        bool
	OTLPEndpoint   string
	SampleRate     float64
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		ServiceName:    TracerName,
		ServiceVersion: "0.1.0",
		Environment:    getEnvOrDefault("OTEL_ENVIRONMENT", "development"),
		Enabled:        os.Getenv("OTEL_ENABLED") == "true" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "",
		OTLPEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		SampleRate:     1.0,
	}
}

// Setup initializes tracing and returns a shutdown function.
func Setup(ctx context.Context, config Config) (func(context.Context) error, error) {
	if !config.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
			semconv.ServiceVersion(config.ServiceVersion),
			attribute.String("environment", config.Environment),
		),
	)
	if err != nil {
		return nil, err
	}

	var exporter sdktrace.SpanExporter
	if config.OTLPEndpoint != "" {
		exporter, err = otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(config.OTLPEndpoint),
			otlptracehttp.WithInsecure(),
		)
	} else {
		exporter, err = stdouttrace.New(stdouttrace.WithPrettyPrint())
	}
	if err != nil {
		return nil, err
	}

	var sampler sdktrace.Sampler
	if config.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else if config.SampleRate <= 0 {
		sampler = sdktrace.NeverSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(config.SampleRate)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}

// Tracer returns the named tracer.
func Tracer() trace.Tracer {
	return otel.Tracer(TracerName)
}

// StartSpan starts a new span.
func StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Tracer().Start(ctx, name, opts...)
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
