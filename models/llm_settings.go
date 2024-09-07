package models

import "gorm.io/gorm"

type LLMSettings struct {
	gorm.Model
	ID                uint                   `gorm:"primaryKey" json:"id"`
	CandidateCount    int                    `json:"candidate_count"`
	FrequencyPenalty  float64                `json:"frequency_penalty"`
	JSONMode          bool                   `json:"json_mode"`
	MaxLength         int                    `json:"max_length"`
	MaxTokens         int                    `json:"max_tokens"`
	Metadata          map[string]interface{} `gorm:"serializer:json" json:"metadata"`
	MinLength         int                    `json:"min_length"`
	ModelName         string                 `json:"model_name"`
	N                 int                    `json:"n"`
	PresencePenalty   float64                `json:"presence_penalty"`
	RepetitionPenalty float64                `json:"repetition_penalty"`
	Seed              int                    `json:"seed"`
	StopWords         []string               `gorm:"serializer:json" json:"stop_words"`
	Temperature       float64                `json:"temperature"`
	TopK              int                    `json:"top_k"`
	TopP              float64                `json:"top_p"`
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
func (ls *LLMSettingsSlice) GetAll(db *gorm.DB) error {
	return db.Find(ls).Error
}

// Get LLMSettings by Model
func (ls *LLMSettings) GetByModel(db *gorm.DB, model string) error {
	return db.Where("model_name = ?", model).First(ls).Error
}

// Search LLMSettings by Model name stub
func (ls *LLMSettingsSlice) SearchByModelStub(db *gorm.DB, modelStub string) error {
	return db.Where("model_name LIKE ?", modelStub+"%").Find(ls).Error
}
