package nativeapi

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckJSONKeys(t *testing.T) {
	ok := []string{
		`{}`, `[]`, `"x"`, `1`, `null`,
		`{"model":"m","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`,
		// The same key in sibling objects is fine.
		`{"messages":[{"content":"a"},{"content":"b"}],"system":[{"text":"x"},{"text":"y"}]}`,
		`{"a":{"b":{"c":[{"d":1},{"d":2}]}},"b":1}`,
		" \n{\"k\": [1, {\"k\": 2}]}\n",
	}
	for _, b := range ok {
		if err := CheckJSONKeys([]byte(b)); err != nil {
			t.Errorf("%s: %v", b, err)
		}
	}

	// Exact duplicates are refused at any depth, whatever foldAt says.
	exact := map[string]string{
		"top level":         `{"model":"ok","model":"evil"}`,
		"escaped":           `{"model":"ok","\u006dodel":"evil"}`,
		"nested message":    `{"messages":[{"role":"user","content":"ok","content":"evil"}]}`,
		"deep in an array":  `[[{"a":1}],[{"b":1,"b":2}]]`,
		"system block text": `{"system":[{"type":"text","text":"a","text":"b"}]}`,
		"tool schema":       `{"tools":[{"input_schema":{"properties":{"id":{},"id":{}}}}]}`,
	}
	for name, b := range exact {
		if err := CheckJSONKeys([]byte(b)); !errors.Is(err, ErrDuplicateKey) {
			t.Errorf("%s: err = %v, want ErrDuplicateKey", name, err)
		}
	}

	// Case variants are refused only in the objects at foldAt.
	foldAt := []string{"", "messages[]", "tool_choice"}
	folded := map[string]string{
		"case variant":     `{"messages":[],"Messages":[]}`,
		"long s":           `{"messages":[],"meſſages":[]}`,
		"kelvin sign":      `{"k":1,"\u212a":2}`,
		"in a message":     `{"messages":[{"role":"user","content":"ok","Content":"evil"}]}`,
		"in tool_choice":   `{"tool_choice":{"type":"any","Type":"none"}}`,
		"after nested obj": `{"a":{"x":1},"A":2}`,
	}
	for name, b := range folded {
		if err := CheckJSONKeys([]byte(b), foldAt...); !errors.Is(err, ErrDuplicateKey) {
			t.Errorf("%s: err = %v, want ErrDuplicateKey", name, err)
		}
		if err := CheckJSONKeys([]byte(b)); err != nil {
			t.Errorf("%s without foldAt: %v", name, err)
		}
	}
	freeForm := map[string]string{
		"schema properties": `{"tools":[{"name":"t","input_schema":{"type":"object","properties":{"id":{"type":"string"},"ID":{"type":"string"}}}}]}`,
		"tool_use input":    `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"f","input":{"Name":"a","name":"b"}}]}]}`,
		"content block":     `{"messages":[{"role":"user","content":[{"type":"text","text":"a","Text":"b"}]}]}`,
		"metadata":          `{"metadata":{"user_id":"a","User_ID":"b"}}`,
		"nested array obj":  `{"messages":[{"role":"user","content":"x","x":[{"k":1,"K":2}]}]}`,
	}
	for name, b := range freeForm {
		if err := CheckJSONKeys([]byte(b), foldAt...); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	bad := []string{``, `{`, `{"a":1,}`, `{"a"}`, `{"a":1} {}`, `[1,2`, `nope`}
	for _, b := range bad {
		err := CheckJSONKeys([]byte(b))
		if err == nil || errors.Is(err, ErrDuplicateKey) {
			t.Errorf("%q: err = %v, want a syntax error", b, err)
		}
	}
}

func TestFoldKey(t *testing.T) {
	for _, pair := range [][2]string{{"messages", "Messages"}, {"messages", "meſſages"}, {"k", "K"}, {"a", "b"}, {"model", "modeI"}} {
		if strings.EqualFold(pair[0], pair[1]) != (FoldKey(pair[0]) == FoldKey(pair[1])) {
			t.Errorf("%q vs %q", pair[0], pair[1])
		}
	}
}
