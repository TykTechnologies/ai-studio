//go:build enterprise

package scripting

import (
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

// tyk.detect and tyk.redact expose the built-in pattern library to scripts.

func TestTykDetect(t *testing.T) {
	out, err := runSource(t, `
		tyk := import("tyk")
		json := import("json")
		found := tyk.detect("mail jane@example.com, key AKIAIOSFODNN7EXAMPLE", ["secrets", "pii"])
		names := []
		for f in found {
			names = append(names, f.detector)
		}
		output := {block: len(found) > 0, payload: input.raw_input, message: string(json.encode(names))}
	`, nil, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if !out.Block {
		t.Error("detect found nothing")
	}
	if !strings.Contains(out.Message, "pii.email") || !strings.Contains(out.Message, "secrets.aws_access_key") {
		t.Errorf("detect returned %s", out.Message)
	}
}

func TestTykDetect_CategoryFilterAndSpans(t *testing.T) {
	out, err := runSource(t, `
		tyk := import("tyk")
		found := tyk.detect("mail jane@example.com", ["secrets"])
		spans := tyk.detect("mail jane@example.com", ["pii.email"])
		output := {block: false, payload: input.raw_input, message: string(len(found)) + "/" + string(spans[0].start) + "-" + string(spans[0].end)}
	`, nil, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if out.Message != "0/5-21" {
		t.Errorf("message = %q, want no secrets and the email span", out.Message)
	}
}

func TestTykRedact_String(t *testing.T) {
	out, err := runSource(t, `
		tyk := import("tyk")
		output := {block: false, payload: tyk.redact("card 4111 1111 1111 1111 ok", ["pii"], "[{{type}}]"), message: ""}
	`, nil, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if out.Payload != "card [PAYMENT_CARD] ok" {
		t.Errorf("Payload = %q", out.Payload)
	}
}

func TestTykRedact_InputRewritesEveryMessage(t *testing.T) {
	raw := `{"model":"gpt-4","messages":[{"role":"system","content":"be nice"},{"role":"user","content":"my key is AKIAIOSFODNN7EXAMPLE"}]}`
	input := &ScriptInput{
		RawInput: raw,
		Messages: []llms.MessageContent{
			{Role: llms.ChatMessageTypeSystem, Parts: []llms.ContentPart{llms.TextPart("be nice")}},
			{Role: llms.ChatMessageTypeHuman, Parts: []llms.ContentPart{llms.TextPart("my key is AKIAIOSFODNN7EXAMPLE")}},
		},
		VendorName: "openai",
	}
	out, err := runSource(t, `
		tyk := import("tyk")
		output := {block: false, payload: tyk.redact(input, ["secrets"], ""), message: ""}
	`, input, nil)
	if err != nil {
		t.Fatalf("RunScript() error = %v", err)
	}
	if strings.Contains(out.Payload, "AKIA") || !strings.Contains(out.Payload, "[REDACTED:AWS_ACCESS_KEY]") || !strings.Contains(out.Payload, `"model":"gpt-4"`) {
		t.Errorf("Payload = %s", out.Payload)
	}
}
