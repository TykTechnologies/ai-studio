package models

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"gorm.io/gorm"
)

// PresentToolOperation is the function name the model calls for generative
// UI. Ships as a built-in client tool (see GetOrCreateDefaultClientTools).
const PresentToolOperation = "present"

// DefaultPresentToolName is the display name of the built-in tool.
const DefaultPresentToolName = "Generative UI"

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

// GetOrCreateDefaultClientTools seeds the built-in client tools on startup,
// the way default LLM configurations are seeded: the generative UI
// ("present") tool exists in every installation and sits in the Default
// tool catalogue, so administrators only have to pick it as a default tool
// of a chat room. Existing installations get it on their next start;
// nothing is touched once the tool exists.
func GetOrCreateDefaultClientTools(db *gorm.DB) error {
	var count int64
	if err := db.Model(&Tool{}).Where("tool_type = ? AND available_operations = ?", ToolTypeClient, PresentToolOperation).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	definition, err := json.Marshal(ClientToolDefinition{UI: ClientToolUI{
		Kind:  ClientToolKindPresent,
		Title: DefaultPresentToolName,
	}})
	if err != nil {
		return fmt.Errorf("marshal present tool definition: %w", err)
	}

	tool := Tool{
		Name:                DefaultPresentToolName,
		Description:         "Present a UI component to the user: dashboards, cards, facts, tables, charts, alerts, lists and forms composed from the built-in vocabulary. Use it whenever a visual layout would be clearer than prose.",
		ToolType:            ToolTypeClient,
		OASSpec:             string(definition),
		AvailableOperations: PresentToolOperation,
		PrivacyScore:        0,
		Active:              true,
		Metadata:            JSONMap{"builtin": true},
	}
	if err := tool.Create(db); err != nil {
		return fmt.Errorf("create present tool: %w", err)
	}

	var catalogue ToolCatalogue
	if err := db.Where("name = ?", DefaultToolCatalogueName).First(&catalogue).Error; err == nil {
		if err := catalogue.AddTool(db, &tool); err != nil {
			return fmt.Errorf("add present tool to default catalogue: %w", err)
		}
	}
	return nil
}
