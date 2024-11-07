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
