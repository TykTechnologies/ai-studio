package anthropicVendor

import (
	"bytes"
	"strings"
	"testing"
)

// Added during independent verification of batch create (ut0.15).

// Escaped spellings of a key are the same key once unescaped, so they count
// as duplicates; the gateway and the vendor must read the same model.
func TestSplitBatchVerify_EscapedDuplicateKeys(t *testing.T) {
	for name, body := range map[string]string{
		"escaped model":     `{"requests":[{"custom_id":"a","params":{"model":"ok","model":"evil","messages":[]}}]}`,
		"escaped params":    `{"requests":[{"custom_id":"a","params":{"model":"ok","messages":[]},"params":{"model":"evil","messages":[]}}]}`,
		"escaped requests":  `{"requests":[{"custom_id":"a","params":{"model":"ok","messages":[]}}],"requests":[]}`,
		"escaped custom_id": `{"requests":[{"custom_id":"a","custom_id":"b","params":{"model":"ok","messages":[]}}]}`,
	} {
		if _, err := splitBatch([]byte(body)); err == nil {
			t.Errorf("%s: accepted %s", name, body)
		}
	}
}

func TestSplitBatchVerify_RejectsShapes(t *testing.T) {
	for name, body := range map[string]string{
		"requests object":    `{"requests":{"custom_id":"a"}}`,
		"params null":        `{"requests":[{"custom_id":"a","params":null}]}`,
		"params array":       `{"requests":[{"custom_id":"a","params":[]}]}`,
		"model number":       `{"requests":[{"custom_id":"a","params":{"model":5,"messages":[]}}]}`,
		"custom_id number":   `{"requests":[{"custom_id":1,"params":{"model":"m","messages":[]}}]}`,
		"request not object": `{"requests":["x"]}`,
		"two bodies":         `{"requests":[{"custom_id":"a","params":{"model":"m"}}]}{}`,
		"not json":           `requests`,
	} {
		if _, err := splitBatch([]byte(body)); err == nil {
			t.Errorf("%s: accepted %s", name, body)
		}
	}
}

// Offsets are byte offsets: multi-byte UTF-8, escapes and odd whitespace
// before an item must not shift the spliced span, and untouched items stay
// byte-identical.
func TestSplitJoinBatchVerify_UnicodeAndWhitespace(t *testing.T) {
	keep := "{ \"model\" : \"m\", \"messages\": [{\"role\":\"user\",\"content\":\"héllo 🌍 \\u00e9 \\\"quoted\\\"\"}] }"
	change := `{"model":"m","messages":[{"role":"user","content":"Zürich SECRET 🔑"}]}`
	body := "\n\t{ \"note\":\"ü🌍\",\r\n \"requests\" : [ {\"custom_id\":\"ké\",\"params\":" + keep + "} ,\n" +
		"{\"params\":" + change + ",\"custom_id\":\"c🔑\"} ] }\n"
	items, err := splitBatch([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Key != "ké" || items[1].Key != "c🔑" {
		t.Fatalf("items %+v", items)
	}
	if string(items[0].Body) != keep || string(items[1].Body) != change {
		t.Fatalf("spans wrong:\n%q\n%q", items[0].Body, items[1].Body)
	}
	items[1].Body = []byte(strings.Replace(change, "SECRET", "[REDACTED]", 1))
	out, err := joinBatch([]byte(body), items)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(body, "SECRET", "[REDACTED]", 1)
	if !bytes.Equal(out, []byte(want)) {
		t.Fatalf("join changed more than the item:\n got %q\nwant %q", out, want)
	}
}
