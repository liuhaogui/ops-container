package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/liuhaogui/ops-container/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func NewTracerProvider(ctx context.Context, cfg config.TelemetryConfig) (*sdktrace.TracerProvider, error) {
	if cfg.Exporter == "noop" {
		tp := sdktrace.NewTracerProvider()
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.TraceContext{})
		return tp, nil
	}

	options := []otlptracegrpc.Option{}
	if cfg.OTLPEndpoint != "" {
		options = append(options, otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint))
	}
	if cfg.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create otlp exporter: %w", err)
	}

	samplerRatio := float64(cfg.SamplingPercent) / 100
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(samplerRatio)),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
		)),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp, nil
}

func ShutdownTracer(ctx context.Context, tp *sdktrace.TracerProvider) error {
	if tp == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return tp.Shutdown(ctx)
}
