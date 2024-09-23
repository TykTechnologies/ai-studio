package models

import "gorm.io/gorm"

type Catalogue struct {
	gorm.Model
	ID   uint   `json:"id" gorm:"primary_key"`
	Name string `json:"name"`
	LLMs []LLM  `gorm:"many2many:catalogue_llms;"`
}

type Catalogues []Catalogue

func NewCatalogue() *Catalogue {
	return &Catalogue{}
}

// Add this new method
func (c *Catalogue) LLMNames() []string {
	names := make([]string, len(c.LLMs))
	for i, llm := range c.LLMs {
		names[i] = llm.Name
	}
	return names
}

func (c *Catalogue) Get(db *gorm.DB, id uint) error {
	return db.Preload("LLMs", func(db *gorm.DB) *gorm.DB {
		return db.Where("active = ?", true)
	}).First(c, id).Error
}

func (c *Catalogue) Create(db *gorm.DB) error {
	return db.Create(c).Error
}

func (c *Catalogue) Update(db *gorm.DB) error {
	return db.Save(c).Error
}

func (c *Catalogue) Delete(db *gorm.DB) error {
	return db.Delete(c).Error
}

func (c *Catalogue) AddLLM(db *gorm.DB, llm *LLM) error {
	return db.Model(c).Association("LLMs").Append(llm)
}

func (c *Catalogue) RemoveLLM(db *gorm.DB, llm *LLM) error {
	return db.Model(c).Association("LLMs").Delete(llm)
}

func (c *Catalogue) GetCatalogueLLMs(db *gorm.DB) error {
	return db.Model(c).Association("LLMs").Find(&c.LLMs)
}

func (c *Catalogues) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Catalogue{})

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

	err := query.Preload("LLMs").Find(c).Error
	return totalCount, totalPages, err
}

func (c *Catalogues) GetByNameStub(db *gorm.DB, stub string) error {
	return db.Preload("LLMs").Where("name LIKE ?", stub+"%").Find(c).Error
}
