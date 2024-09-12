package models

import (
	"time"

	"gorm.io/gorm"
)

// CMessage is the GORM model for chat messages
type CMessage struct {
	gorm.Model
	ID        uint   `gorm:"primaryKey"`
	Session   string `gorm:"index"`
	Content   []byte
	CreatedAt time.Time
}
