// Package tracing 基于 OpenTelemetry SDK 提供统一的分布式追踪初始化与 span 工具。
package tracing

import (
	"context"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "stableagent"

// Setup 初始化全局 TracerProvider 和 W3C 传播器。
// 未设置 OTEL_EXPORTER_OTLP_ENDPOINT 时不导出 span(保持 no-op provider),业务照常运行。
func Setup(ctx context.Context, serviceName string, env string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}
	// otlptracehttp 直接读取 OTEL_EXPORTER_OTLP_* 环境变量。
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	resource, err := sdkresource.Merge(sdkresource.Default(), sdkresource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		attribute.String("deployment.environment", env),
	))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource),
	)
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

// Tracer 返回统一命名的业务 tracer。
func Tracer() trace.Tracer {
	return otel.Tracer(tracerName)
}

// StartSpan 创建业务 span 并附带 task/tenant 维度。
func StartSpan(ctx context.Context, name string, taskID string, tenantID string, extra ...attribute.KeyValue) (context.Context, trace.Span) {
	attrs := append([]attribute.KeyValue{
		attribute.String("stableagent.task_id", taskID),
		attribute.String("stableagent.tenant_id", tenantID),
	}, extra...)
	return Tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

// Inject 将当前 trace 上下文注入 HTTP header,用于服务间传播。
func Inject(ctx context.Context, header propagation.HeaderCarrier) {
	otel.GetTextMapPropagator().Inject(ctx, header)
}

// Extract 从 HTTP header 提取 trace 上下文。
func Extract(ctx context.Context, header propagation.HeaderCarrier) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, header)
}
