package services

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

type Service struct {
	DB     *gorm.DB
	Budget *BudgetService
}

func NewService(db *gorm.DB) *Service {
	secrets.SetDBRef(db)
	return &Service{
		DB: db,
	}
}

func (s *Service) GetDB() *gorm.DB {
	return s.DB
}
