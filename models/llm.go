package models

import (
	"time"

	"gorm.io/gorm"
)

type Vendor string

type LLM struct {
	gorm.Model
	ID               uint   `json:"id" gorm:"primary_key"`
	Name             string `json:"name"`
	APIKey           string `json:"api_key"`
	APIEndpoint      string `json:"api_endpoint"`
	DefaultModel     string `json:"default_model"`
	PrivacyScore     int    `json:"privacy_score"`
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`
	LogoURL          string `json:"logo"`
	Vendor           Vendor `json:"vendor"`
	Active           bool   `json:"active"`
	// Budget
	MonthlyBudget   *float64   `json:"monthly_budget" gorm:"column:monthly_budget"`
	BudgetStartDate *time.Time `json:"budget_start_date" gorm:"column:budget_start_date"`
	// Hub-and-Spoke Configuration
	Namespace       string     `json:"namespace" gorm:"default:'';index:idx_llm_namespace"`

	Filters       []*Filter `json:"filters" gorm:"many2many:llm_filters;"`
	Plugins       []*Plugin `json:"plugins" gorm:"many2many:llm_plugins;"`
	AllowedModels []string  `json:"allowed_models" gorm:"serializer:json"`

	// Plugin-stored metadata
	Metadata JSONMap `json:"metadata" gorm:"type:json"`
}

const (
	OPENAI      Vendor = "openai"
	ANTHROPIC   Vendor = "anthropic"
	VERTEX      Vendor = "vertex"
	GOOGLEAI    Vendor = "google_ai"
	HUGGINGFACE Vendor = "huggingface"
	OLLAMA      Vendor = "ollama"
	MOCK_VENDOR Vendor = "mock"
)

type LLMs []LLM

func NewLLM() *LLM {
	return &LLM{}
}

func (l *LLM) Get(db *gorm.DB, id uint) error {
	return db.Preload("Filters").Preload("Plugins").First(l, id).Error
}

func (l *LLM) Create(db *gorm.DB) error {
	tx := db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := tx.Error; err != nil {
		return err
	}
	if err := tx.Create(l).Error; err != nil {
		tx.Rollback()
		return err
	}
	if len(l.Filters) > 0 {
		if err := tx.Model(l).Association("Filters").Replace(l.Filters); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

func (l *LLM) Update(db *gorm.DB) error {
	if err := db.Save(l).Error; err != nil {
		return err
	}
	return db.Model(l).Association("Filters").Replace(l.Filters)
}

func (l *LLM) Delete(db *gorm.DB) error {
	return db.Delete(l).Error
}

func (l *LLM) GetByName(db *gorm.DB, name string) error {
	return db.Preload("Filters").Preload("Plugins").Where("name = ?", name).First(l).Error
}

func (l *LLMs) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&LLM{}).Preload("Filters").Preload("Plugins")
	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}
	var totalPages int
	if pageSize > 0 {
		totalPages = int(totalCount) / pageSize
		if int(totalCount)%pageSize != 0 {
			totalPages++
		}
	}
	if !all && pageSize > 0 {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}
	err := query.Find(l).Error
	return totalCount, totalPages, err
}

func (l *LLMs) GetByNameStub(db *gorm.DB, stub string) error {
	// Use single query with preloading for better performance
	return db.Preload("Filters").Preload("Plugins").Where("name LIKE ?", stub+"%").Find(l).Error
}

func (l *LLMs) GetByMaxPrivacyScore(db *gorm.DB, score int) error {
	return db.Preload("Filters").Preload("Plugins").Where("privacy_score <= ?", score).Find(l).Error
}

func (l *LLMs) GetByMinPrivacyScore(db *gorm.DB, score int) error {
	return db.Preload("Filters").Preload("Plugins").Where("privacy_score >= ?", score).Find(l).Error
}

func (l *LLMs) GetByPrivacyScoreRange(db *gorm.DB, min, max int) error {
	return db.Preload("Filters").Preload("Plugins").Where("privacy_score BETWEEN ? AND ?", min, max).Find(l).Error
}

func (l *LLMs) GetActiveLLMs(db *gorm.DB) error {
	return db.Preload("Filters").Preload("Plugins").Where("active = ?", true).Find(l).Error
}

func (l *LLMs) GetLLMCount(db *gorm.DB) (int64, error) {
	var count int64
	err := db.Model(&LLM{}).Count(&count).Error

	return count, err
}

func GetTotalTokens(db *gorm.DB) (int64, error) {
	var totalTokens int64
	err := db.Model(&LLMChatRecord{}).
		Select("COALESCE(SUM(total_tokens), 0) as total_tokens").
		Scan(&totalTokens).Error

	return totalTokens, err
}

func GetTotalTokensByInteractionType(db *gorm.DB, interactionType InteractionType) (int64, error) {
	var totalTokens int64
	err := db.Model(&LLMChatRecord{}).
		Where("interaction_type = ?", interactionType).
		Select("COALESCE(SUM(total_tokens), 0) as total_tokens").
		Scan(&totalTokens).Error

	return totalTokens, err
}
