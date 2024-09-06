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

func (c *Catalogue) Get(db *gorm.DB, id uint) error {
	return db.Preload("LLMs").First(c, id).Error
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

func (c *Catalogues) GetAll(db *gorm.DB) error {
	return db.Preload("LLMs").Find(c).Error
}

func (c *Catalogues) GetByNameStub(db *gorm.DB, stub string) error {
	return db.Preload("LLMs").Where("name LIKE ?", stub+"%").Find(c).Error
}
