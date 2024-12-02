package services

import (
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"gorm.io/gorm"
)

type Service struct {
	DB *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	secrets.SetDBRef(db)
	return &Service{DB: db}
}
