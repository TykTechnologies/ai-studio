package models

import "gorm.io/gorm"

type Vendor string

type LLM struct {
	gorm.Model
	ID          uint   `json:"id" gorm:"primary_key"`
	Name        string `json:"name"`
	APIKey      string `json:"api_key`
	APIEndpoint string `json:"api_endpoint"` // The endpoint to actually access the LLM with Midsommar

	PrivacyScore     int    `json:"privacy_score"`
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`

	LogoURL string `json:"logo"`
	Vendor  Vendor `json:"vendor"`
	Active  bool   `json:"active"`
}

const (
	OPENAI    Vendor = "openai"
	ANTHROPIC Vendor = "anthropic"
	// BEDROCK     Vendor = "bedrock"
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
	return db.First(l, id).Error
}

func (l *LLM) Create(db *gorm.DB) error {
	return db.Create(l).Error
}

func (l *LLM) Update(db *gorm.DB) error {
	return db.Save(l).Error
}

func (l *LLM) Delete(db *gorm.DB) error {
	return db.Delete(l).Error
}

func (l *LLM) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).First(l).Error
}

func (l *LLMs) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&LLM{})

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

	err := query.Find(l).Error
	return totalCount, totalPages, err
}

func (l *LLMs) GetByNameStub(db *gorm.DB, stub string) error {
	return db.Where("name LIKE ?", stub+"%").Find(l).Error
}

func (l *LLMs) GetByMaxPrivacyScore(db *gorm.DB, score int) error {
	return db.Where("privacy_score <= ?", score).Find(l).Error
}

func (l *LLMs) GetByMinPrivacyScore(db *gorm.DB, score int) error {
	return db.Where("privacy_score >= ?", score).Find(l).Error
}

func (l *LLMs) GetByPrivacyScoreRange(db *gorm.DB, min, max int) error {
	return db.Where("privacy_score BETWEEN ? AND ?", min, max).Find(l).Error
}

func (l *LLMs) GetActiveLLMs(db *gorm.DB) error {
	return db.Where("active = ?", true).Find(l).Error
}
