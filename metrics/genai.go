package metrics

import (
	"context"
	"os"
	"strconv"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
)

// This file implements the OpenTelemetry GenAI semantic conventions (v1.37.0)
// alongside the aistudio_* instruments in metrics.go.
//
// Upstream status: the gen_ai.* conventions are still marked "Development", not
// Stable, so these names may shift. Only metrics with a genuine counterpart in
// the conventions are mirrored here — cost, policy blocks, compliance events and
// in-flight requests have none and stay under aistudio_* alone.

// EnvLegacyNames ("METRICS_LEGACY_NAMES") controls whether the pre-conventions
// aistudio_* instruments are emitted alongside the gen_ai.* ones. Defaults to
// true so existing dashboards keep working; set to "false" once they have been
// migrated.
const EnvLegacyNames = "METRICS_LEGACY_NAMES"

// GenAI semantic convention attribute keys.
const (
	attrOperationName = "gen_ai.operation.name"
	attrProviderName  = "gen_ai.provider.name"
	attrRequestModel  = "gen_ai.request.model"
	attrTokenType     = "gen_ai.token.type"
	attrErrorType     = "error.type"
)

// GenAI semantic convention operation names.
const (
	OperationChat        = "chat"
	OperationExecuteTool = "execute_tool"
)

// Advisory bucket boundaries from the GenAI metric definitions.
var (
	genaiDurationBuckets = []float64{0.01, 0.02, 0.04, 0.08, 0.16, 0.32, 0.64, 1.28, 2.56, 5.12, 10.24, 20.48, 40.96, 81.92}
	genaiTPOTBuckets     = []float64{0.01, 0.025, 0.05, 0.075, 0.1, 0.15, 0.2, 0.3, 0.4, 0.5, 0.75, 1.0, 2.5}
	genaiTokenBuckets    = []float64{1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864}
)

var (
	genaiRequestDuration   otelmetric.Float64Histogram
	genaiTimeToFirstToken  otelmetric.Float64Histogram
	genaiTimePerOutputTok  otelmetric.Float64Histogram
	genaiTokenUsage        otelmetric.Int64Histogram
	genaiOperationDuration otelmetric.Float64Histogram

	legacyNames bool
)

// initGenAI registers the gen_ai.* instruments. Called from Init.
func initGenAI(meter otelmetric.Meter) {
	legacyNames = os.Getenv(EnvLegacyNames) != "false"

	var err error

	genaiRequestDuration, err = meter.Float64Histogram("gen_ai.server.request.duration",
		otelmetric.WithDescription("Generative AI server request duration"),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(genaiDurationBuckets...),
	)
	if err != nil {
		panic("failed to create gen_ai.server.request.duration histogram: " + err.Error())
	}

	genaiTimeToFirstToken, err = meter.Float64Histogram("gen_ai.server.time_to_first_token",
		otelmetric.WithDescription("Time to generate the first token for successful streaming responses"),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(genaiDurationBuckets...),
	)
	if err != nil {
		panic("failed to create gen_ai.server.time_to_first_token histogram: " + err.Error())
	}

	genaiTimePerOutputTok, err = meter.Float64Histogram("gen_ai.server.time_per_output_token",
		otelmetric.WithDescription("Time per output token generated after the first token"),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(genaiTPOTBuckets...),
	)
	if err != nil {
		panic("failed to create gen_ai.server.time_per_output_token histogram: " + err.Error())
	}

	genaiTokenUsage, err = meter.Int64Histogram("gen_ai.client.token.usage",
		otelmetric.WithDescription("Number of input and output tokens used"),
		otelmetric.WithUnit("{token}"),
		otelmetric.WithExplicitBucketBoundaries(genaiTokenBuckets...),
	)
	if err != nil {
		panic("failed to create gen_ai.client.token.usage histogram: " + err.Error())
	}

	genaiOperationDuration, err = meter.Float64Histogram("gen_ai.client.operation.duration",
		otelmetric.WithDescription("Generative AI operation duration"),
		otelmetric.WithUnit("s"),
		otelmetric.WithExplicitBucketBoundaries(genaiDurationBuckets...),
	)
	if err != nil {
		panic("failed to create gen_ai.client.operation.duration histogram: " + err.Error())
	}
}

// GenAIProviderName maps an AI Studio vendor identifier to the well-known
// gen_ai.provider.name value. Vendors with no registered value pass through
// unchanged.
func GenAIProviderName(vendor string) string {
	switch vendor {
	case "bedrock":
		return "aws.bedrock"
	case "vertex":
		return "gcp.vertex_ai"
	case "google_ai":
		return "gcp.gemini"
	default:
		return vendor
	}
}

// genaiTokenTypeOf maps AI Studio token categories onto gen_ai.token.type. The
// conventions define only "input" and "output"; the cache categories have no
// registered value and pass through so the data is not lost.
func genaiTokenTypeOf(tokenType string) string {
	switch tokenType {
	case "prompt":
		return "input"
	case "completion":
		return "output"
	default:
		return tokenType
	}
}

// errorTypeOf returns the error.type attribute value for an HTTP status code,
// or "" when the status does not represent an error.
func errorTypeOf(statusCode int) string {
	if statusCode < 400 {
		return ""
	}
	return strconv.Itoa(statusCode)
}

// recordGenAIRequestDuration mirrors ObserveRequestDuration onto
// gen_ai.server.request.duration. error.type is set only when the request
// terminated in an error, as the conventions require.
func recordGenAIRequestDuration(ctx context.Context, vendor, model string, seconds float64, statusCode int) {
	attrs := []attribute.KeyValue{
		attribute.String(attrOperationName, OperationChat),
		attribute.String(attrProviderName, GenAIProviderName(vendor)),
		attribute.String(attrRequestModel, model),
	}
	if errType := errorTypeOf(statusCode); errType != "" {
		attrs = append(attrs, attribute.String(attrErrorType, errType))
	}
	genaiRequestDuration.Record(ctx, seconds, otelmetric.WithAttributes(attrs...))
}

// recordGenAITokens mirrors RecordTokens onto gen_ai.client.token.usage.
func recordGenAITokens(ctx context.Context, vendor, model, tokenType string, count int) {
	genaiTokenUsage.Record(ctx, int64(count),
		otelmetric.WithAttributes(
			attribute.String(attrOperationName, OperationChat),
			attribute.String(attrProviderName, GenAIProviderName(vendor)),
			attribute.String(attrRequestModel, model),
			attribute.String(attrTokenType, genaiTokenTypeOf(tokenType)),
		),
	)
}

// recordGenAIToolDuration mirrors ObserveToolDuration onto
// gen_ai.client.operation.duration.
func recordGenAIToolDuration(ctx context.Context, toolName string, seconds float64) {
	genaiOperationDuration.Record(ctx, seconds,
		otelmetric.WithAttributes(
			attribute.String(attrOperationName, OperationExecuteTool),
			attribute.String("gen_ai.tool.name", toolName),
		),
	)
}

// ObserveTimeToFirstToken records the latency from request start to the first
// streamed chunk. Streaming responses only.
func ObserveTimeToFirstToken(ctx context.Context, vendor, model string, seconds float64) {
	if !initialized.Load() {
		return
	}
	genaiTimeToFirstToken.Record(ctx, seconds,
		otelmetric.WithAttributes(
			attribute.String(attrOperationName, OperationChat),
			attribute.String(attrProviderName, GenAIProviderName(vendor)),
			attribute.String(attrRequestModel, model),
		),
	)
}

// ObserveTimePerOutputToken records the mean inter-token latency after the first
// token, given the elapsed time between the first token and the end of the
// stream. Streaming responses only; a call with no output tokens is dropped
// rather than recorded as zero.
func ObserveTimePerOutputToken(ctx context.Context, vendor, model string, secondsAfterFirstToken float64, outputTokens int) {
	if !initialized.Load() || outputTokens <= 0 || secondsAfterFirstToken <= 0 {
		return
	}
	genaiTimePerOutputTok.Record(ctx, secondsAfterFirstToken/float64(outputTokens),
		otelmetric.WithAttributes(
			attribute.String(attrOperationName, OperationChat),
			attribute.String(attrProviderName, GenAIProviderName(vendor)),
			attribute.String(attrRequestModel, model),
		),
	)
}
