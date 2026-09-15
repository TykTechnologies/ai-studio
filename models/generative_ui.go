package models

import (
	_ "embed"
	"encoding/json"
	"sync"
)

// generativeUIPresentSchema is the JSON schema of the built-in generative UI
// ("present") client tool: the component vocabulary the chat UI can draw
// (cards, facts, tables, charts, forms, ...). It is generated from the
// installed @assistant-ui/react-generative-ui library so the model and the
// renderer always agree on the vocabulary:
//
//	cd ui/admin-frontend && node scripts/gen-present-schema.mjs
//
//go:embed generative_ui_present_schema.json
var generativeUIPresentSchema []byte

// PresentToolSpec is the model-facing half of the generative UI tool.
type PresentToolSpec struct {
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

var (
	presentSpecOnce sync.Once
	presentSpec     PresentToolSpec
	presentSpecErr  error
)

// PresentToolSchema returns the embedded generative UI tool definition. The
// parameters map is shared; callers must not mutate it.
func PresentToolSchema() (PresentToolSpec, error) {
	presentSpecOnce.Do(func() {
		presentSpecErr = json.Unmarshal(generativeUIPresentSchema, &presentSpec)
	})
	return presentSpec, presentSpecErr
}
