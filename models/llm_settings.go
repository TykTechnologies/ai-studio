package models

import (
	"context"

	"github.com/tmc/langchaingo/llms"
	"gorm.io/gorm"
)

type LLMSettings struct {
	gorm.Model
	ID                uint                   `gorm:"primaryKey" json:"id"`
	MaxLength         int                    `json:"max_length"`
	MaxTokens         int                    `json:"max_tokens"`
	Metadata          map[string]interface{} `gorm:"serializer:json" json:"metadata"`
	MinLength         int                    `json:"min_length"`
	ModelName         string                 `json:"model_name"`
	RepetitionPenalty float64                `json:"repetition_penalty"`
	Seed              int                    `json:"seed"`
	StopWords         []string               `gorm:"serializer:json" json:"stop_words"`
	Temperature       float64                `json:"temperature"`
	TopK              int                    `json:"top_k"`
	TopP              float64                `json:"top_p"`
	SystemPrompt      string                 `json:"system_prompt"`
}

type LLMSettingsSlice []LLMSettings

func NewLLMSettings() *LLMSettings {
	return &LLMSettings{}
}

// Create a new LLMSettings
func (ls *LLMSettings) Create(db *gorm.DB) error {
	return db.Create(ls).Error
}

// Get an LLMSettings by ID
func (ls *LLMSettings) Get(db *gorm.DB, id uint) error {
	return db.First(ls, id).Error
}

// Update an existing LLMSettings
func (ls *LLMSettings) Update(db *gorm.DB) error {
	return db.Save(ls).Error
}

// Delete an LLMSettings
func (ls *LLMSettings) Delete(db *gorm.DB) error {
	return db.Delete(ls).Error
}

// Get all LLMSettings
func (ls *LLMSettingsSlice) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&LLMSettings{})

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Find(ls).Error
	return totalCount, totalPages, err
}

// Get LLMSettings by Model
func (ls *LLMSettings) GetByModel(db *gorm.DB, model string) error {
	return db.Where("model_name = ?", model).First(ls).Error
}

// Search LLMSettings by Model name stub
func (ls *LLMSettingsSlice) SearchByModelStub(db *gorm.DB, modelStub string) error {
	return db.Where("model_name LIKE ?", modelStub+"%").Find(ls).Error
}

// DefaultLLMSettings returns a slice of default LLM settings for popular SOTA models.
// These settings are based on official vendor documentation as of December 2025.
func DefaultLLMSettings() []LLMSettings {
	return []LLMSettings{
		// OpenAI GPT-5 Family (December 2025) - temperature=1.0 is default
		// OpenAI recommends using temperature OR top_p, not both. TopP=0 means not sent.
		{ModelName: "gpt-5.2", Temperature: 1.0, MaxTokens: 128000, MaxLength: 400000},
		{ModelName: "gpt-5.1", Temperature: 1.0, MaxTokens: 128000, MaxLength: 400000},
		{ModelName: "gpt-5", Temperature: 1.0, MaxTokens: 128000, MaxLength: 400000},
		{ModelName: "gpt-5-mini", Temperature: 1.0, MaxTokens: 65536, MaxLength: 400000},
		{ModelName: "gpt-5-nano", Temperature: 1.0, MaxTokens: 32768, MaxLength: 200000},
		// OpenAI GPT-4 Family (Legacy)
		{ModelName: "gpt-4.1", Temperature: 1.0, MaxTokens: 32768, MaxLength: 1047576},
		{ModelName: "gpt-4o", Temperature: 1.0, MaxTokens: 16384, MaxLength: 128000},
		{ModelName: "gpt-4o-mini", Temperature: 1.0, MaxTokens: 16384, MaxLength: 128000},
		// Anthropic Claude 4.6 Family - temperature=1.0 is default, 64K max output
		{ModelName: "claude-opus-4-6", Temperature: 1.0, MaxTokens: 64000, TopP: 1.0, MaxLength: 200000},
		// Anthropic Claude 4.5 Family (December 2025) - temperature=1.0 is default, 64K max output
		{ModelName: "claude-opus-4-5-20251101", Temperature: 1.0, MaxTokens: 64000, TopP: 1.0, MaxLength: 200000},
		{ModelName: "claude-sonnet-4-5-20250929", Temperature: 1.0, MaxTokens: 64000, TopP: 1.0, MaxLength: 200000},
		{ModelName: "claude-haiku-4-5-20251001", Temperature: 1.0, MaxTokens: 64000, TopP: 1.0, MaxLength: 200000},
		// Anthropic Claude 4 Family
		{ModelName: "claude-sonnet-4-20250514", Temperature: 1.0, MaxTokens: 16384, TopP: 1.0, MaxLength: 200000},
		{ModelName: "claude-opus-4-20250514", Temperature: 1.0, MaxTokens: 32768, TopP: 1.0, MaxLength: 200000},
		// Anthropic Claude 3.5 Family (Legacy)
		{ModelName: "claude-3-5-sonnet-20241022", Temperature: 1.0, MaxTokens: 8192, TopP: 1.0, MaxLength: 200000},
		{ModelName: "claude-3-5-haiku-20241022", Temperature: 1.0, MaxTokens: 8192, TopP: 1.0, MaxLength: 200000},
		// Google Gemini 3 Family (December 2025) - temperature=1.0 is default, top_k=40 is typical
		{ModelName: "gemini-3-pro-preview", Temperature: 1.0, MaxTokens: 65536, TopP: 0.95, TopK: 40, MaxLength: 1048576},
		// Google Gemini 2.5 Family
		{ModelName: "gemini-2.5-pro", Temperature: 1.0, MaxTokens: 65536, TopP: 0.95, TopK: 40, MaxLength: 1048576},
		{ModelName: "gemini-2.5-flash", Temperature: 1.0, MaxTokens: 65536, TopP: 0.95, TopK: 40, MaxLength: 1048576},
		{ModelName: "gemini-2.5-flash-lite", Temperature: 1.0, MaxTokens: 65536, TopP: 0.95, TopK: 40, MaxLength: 1048576},
		// Google Gemini 2.0 Family (Legacy)
		{ModelName: "gemini-2.0-flash", Temperature: 1.0, MaxTokens: 8192, TopP: 0.95, TopK: 40, MaxLength: 1048576},
		{ModelName: "gemini-2.0-flash-lite", Temperature: 1.0, MaxTokens: 8192, TopP: 0.95, TopK: 40, MaxLength: 1048576},
		// Meta LLama Models
		{ModelName: "llama-3.3-70b", Temperature: 1.0, MaxTokens: 4096, TopP: 0.9, TopK: 40, MaxLength: 128000},
		{ModelName: "llama-3.2-90b", Temperature: 1.0, MaxTokens: 4096, TopP: 0.9, TopK: 40, MaxLength: 128000},
	}
}

// GetOrCreateDefaultLLMSettings ensures default LLM settings exist in the database.
// This function only seeds defaults if the llm_settings table is empty,
// preventing overwriting of user customizations.
func GetOrCreateDefaultLLMSettings(db *gorm.DB) error {
	var count int64
	if err := db.Model(&LLMSettings{}).Count(&count).Error; err != nil {
		return err
	}

	// Only seed if table is empty
	if count == 0 {
		defaults := DefaultLLMSettings()
		for _, setting := range defaults {
			if err := db.Create(&setting).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (ls *LLMSettings) GenerateOptionsFromSettings(tools []llms.Tool, mode string, streamingFunc func(ctx context.Context, chunk []byte) error) []llms.CallOption {
	var callOptions = make([]llms.CallOption, 0)

	if ls.MaxLength > 0 {
		callOptions = append(callOptions, llms.WithMaxLength(ls.MaxLength))
	}
	if ls.MaxTokens > 0 {
		callOptions = append(callOptions, llms.WithMaxTokens(ls.MaxTokens))
	}
	if ls.MinLength > 0 {
		callOptions = append(callOptions, llms.WithMinLength(ls.MinLength))
	}

	if ls.RepetitionPenalty > 0 {
		callOptions = append(callOptions, llms.WithRepetitionPenalty(ls.RepetitionPenalty))
	}
	if ls.Seed > 0 {
		callOptions = append(callOptions, llms.WithSeed(ls.Seed))
	}
	if len(ls.StopWords) > 0 {
		callOptions = append(callOptions, llms.WithStopWords(ls.StopWords))
	}
	if ls.Temperature > 0 {
		callOptions = append(callOptions, llms.WithTemperature(ls.Temperature))
	}
	if ls.TopK > 0 {
		callOptions = append(callOptions, llms.WithTopK(ls.TopK))
	}
	if ls.TopP > 0 {
		callOptions = append(callOptions, llms.WithTopP(ls.TopP))
	}

	if mode == "stream" {
		callOptions = append(callOptions, llms.WithStreamingFunc(streamingFunc))
	}

	if len(tools) > 0 {
		callOptions = append(callOptions, llms.WithTools(tools))
	}

	return callOptions
}
