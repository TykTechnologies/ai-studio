package proxy

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/pkg/requestformat"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream"
	"github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream/eventstreamapi"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
)

// encodeBedrockEventStreamMessage encodes a ConverseStream SDK event back to
// the AWS binary event stream format that native clients (boto3, etc.) expect.
func encodeBedrockEventStreamMessage(encoder *eventstream.Encoder, w io.Writer, event types.ConverseStreamOutput) error {
	eventType, payload, err := marshalEventForBinaryStream(event)
	if err != nil {
		return fmt.Errorf("marshal event %T: %w", event, err)
	}

	msg := eventstream.Message{
		Headers: eventstream.Headers{
			{Name: eventstreamapi.MessageTypeHeader, Value: eventstream.StringValue(eventstreamapi.EventMessageType)},
			{Name: eventstreamapi.EventTypeHeader, Value: eventstream.StringValue(eventType)},
			{Name: eventstreamapi.ContentTypeHeader, Value: eventstream.StringValue("application/json")},
		},
		Payload: payload,
	}

	return encoder.Encode(w, msg)
}

// marshalEventForBinaryStream returns the event type string and JSON payload
// matching the exact wire format that AWS Bedrock sends in the binary event stream.
// We cannot use json.Marshal on SDK structs directly because they don't have json tags
// matching the wire format — the SDK uses custom serializers internally.
func marshalEventForBinaryStream(event types.ConverseStreamOutput) (string, []byte, error) {
	switch v := event.(type) {
	case *types.ConverseStreamOutputMemberMessageStart:
		payload, err := json.Marshal(map[string]any{
			"role": string(v.Value.Role),
		})
		return "messageStart", payload, err
	case *types.ConverseStreamOutputMemberContentBlockStart:
		startMap := map[string]any{
			"contentBlockIndex": aws.ToInt32(v.Value.ContentBlockIndex),
			"start":             map[string]any{},
		}
		if v.Value.Start != nil {
			if toolUse, ok := v.Value.Start.(*types.ContentBlockStartMemberToolUse); ok {
				startMap["start"] = map[string]any{
					"toolUse": map[string]any{
						"toolUseId": toolUse.Value.ToolUseId,
						"name":      toolUse.Value.Name,
					},
				}
			}
		}
		payload, err := json.Marshal(startMap)
		return "contentBlockStart", payload, err
	case *types.ConverseStreamOutputMemberContentBlockDelta:
		delta := map[string]any{}
		if v.Value.Delta != nil {
			if textDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
				delta["text"] = textDelta.Value
			} else if toolDelta, ok := v.Value.Delta.(*types.ContentBlockDeltaMemberToolUse); ok {
				delta["toolUse"] = map[string]any{"input": toolDelta.Value.Input}
			}
		}
		payload, err := json.Marshal(map[string]any{
			"contentBlockIndex": aws.ToInt32(v.Value.ContentBlockIndex),
			"delta":             delta,
		})
		return "contentBlockDelta", payload, err
	case *types.ConverseStreamOutputMemberContentBlockStop:
		payload, err := json.Marshal(map[string]any{
			"contentBlockIndex": aws.ToInt32(v.Value.ContentBlockIndex),
		})
		return "contentBlockStop", payload, err
	case *types.ConverseStreamOutputMemberMessageStop:
		payload, err := json.Marshal(map[string]any{
			"stopReason": string(v.Value.StopReason),
		})
		return "messageStop", payload, err
	case *types.ConverseStreamOutputMemberMetadata:
		result := map[string]any{}
		if v.Value.Usage != nil {
			result["usage"] = map[string]any{
				"inputTokens":  aws.ToInt32(v.Value.Usage.InputTokens),
				"outputTokens": aws.ToInt32(v.Value.Usage.OutputTokens),
				"totalTokens":  aws.ToInt32(v.Value.Usage.TotalTokens),
			}
		}
		if v.Value.Metrics != nil {
			result["metrics"] = map[string]any{
				"latencyMs": aws.ToInt64(v.Value.Metrics.LatencyMs),
			}
		}
		payload, err := json.Marshal(result)
		return "metadata", payload, err
	default:
		return "unknown", []byte("{}"), nil
	}
}

// marshalEventPayload serializes an event to JSON for filter evaluation.
func marshalEventPayload(event types.ConverseStreamOutput) ([]byte, error) {
	_, payload, err := marshalEventForBinaryStream(event)
	return payload, err
}

// writeEventStreamError writes an error message as a binary event stream exception.
func writeEventStreamError(encoder *eventstream.Encoder, w io.Writer, errMsg string) {
	payload, _ := json.Marshal(map[string]string{"message": errMsg})
	msg := eventstream.Message{
		Headers: eventstream.Headers{
			{Name: eventstreamapi.MessageTypeHeader, Value: eventstream.StringValue(eventstreamapi.ExceptionMessageType)},
			{Name: eventstreamapi.ExceptionTypeHeader, Value: eventstream.StringValue("validationException")},
			{Name: eventstreamapi.ContentTypeHeader, Value: eventstream.StringValue("application/json")},
		},
		Payload: payload,
	}
	encoder.Encode(w, msg)
}

// --- Helper types and functions ---

// ExtractBedrockModelIDFromPath extracts the model ID from a Bedrock URL path.
// Native Bedrock API paths contain /model/{modelId}/converse or /model/{modelId}/converse-stream.
// Pass the escaped path (r.URL.EscapedPath()): the AWS SDKs escape the model
// ID, so an ARN or inference profile arrives as arn%3Aaws%3Abedrock%3A...%2F...
// and comes back unescaped and whole (requestformat.Model).
func ExtractBedrockModelIDFromPath(path string) string {
	return requestformat.Model(string(models.BEDROCK), path, nil)
}

// extractTextFromStreamEvent extracts text content from a ConverseStream event for response filtering.
func extractTextFromStreamEvent(event types.ConverseStreamOutput) string {
	if delta, ok := event.(*types.ConverseStreamOutputMemberContentBlockDelta); ok {
		if textDelta, ok := delta.Value.Delta.(*types.ContentBlockDeltaMemberText); ok {
			return textDelta.Value
		}
	}
	return ""
}
