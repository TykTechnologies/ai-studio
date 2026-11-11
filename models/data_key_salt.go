package models

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// DataKeySalt is the database's salt for data-key encryption
// (secrets.SealWithDataKey): App credentials are encrypted under one key
// derived from TYK_AI_SECRET_KEY and this salt. One row, id 1.
//
// It is not a secret: it only makes the derived key specific to this
// database. It is part of the database: every replica reads the same row,
// and a backup carries it. Without it nothing sealed under it can be read,
// so it is created once (EnsureDataKeySalt, under the migration lock) and
// never replaced: if it is missing while credentials sealed under it exist,
// Studio refuses to start rather than make a new one.
type DataKeySalt struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Salt      []byte    `json:"-" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
}

const dataKeySaltRowID = 1

// ErrDataKeySaltLost: credentials sealed under the database's salt exist,
// and the salt does not.
var ErrDataKeySaltLost = errors.New("the database's data key salt (table data_key_salts) is missing, but App credentials encrypted with it exist: they cannot be read without it. Restore the data_key_salts row from the backup the credentials came from; a new salt is never generated in its place")

// dataKeySalts caches each database's salt for the life of the process: it
// never changes. Keyed by the *gorm.Config every session and transaction of
// one opened database shares.
var dataKeySalts sync.Map // *gorm.Config -> []byte

// EnsureDataKeySalt creates the database's salt if it has none and returns
// it. Call it under the migration lock, before credentials are migrated. It
// is safe beside another replica doing the same: the first insert wins and
// both read that row. It refuses, with ErrDataKeySaltLost, to create a salt
// when credentials sealed under a lost one exist.
func EnsureDataKeySalt(db *gorm.DB) ([]byte, error) {
	if salt, err := loadDataKeySalt(db); err != nil || salt != nil {
		return salt, err
	}
	db = db.Session(&gorm.Session{NewDB: true})
	if !db.Migrator().HasTable(&DataKeySalt{}) {
		// A database Studio did not migrate (a test's own tables).
		if err := db.AutoMigrate(&DataKeySalt{}); err != nil {
			return nil, fmt.Errorf("create the data key salt table: %w", err)
		}
	}
	if db.Migrator().HasTable("credentials") {
		var sealed int64
		if err := db.Table("credentials").Where("secret LIKE ?", secrets.DataKeyPrefix+"%").Limit(1).Count(&sealed).Error; err != nil {
			return nil, fmt.Errorf("look for encrypted credentials: %w", err)
		}
		if sealed > 0 {
			return nil, ErrDataKeySaltLost
		}
	}
	salt := make([]byte, secrets.DataKeySaltLength)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate the data key salt: %w", err)
	}
	row := DataKeySalt{ID: dataKeySaltRowID, Salt: salt, CreatedAt: time.Now()}
	if err := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoNothing: true}).Create(&row).Error; err != nil {
		return nil, fmt.Errorf("store the data key salt: %w", err)
	}
	// Read back what is stored: another replica may have inserted first.
	salt, err := loadDataKeySalt(db)
	if err == nil && salt == nil {
		err = errors.New("the data key salt could not be read back after it was stored")
	}
	return salt, err
}

// loadDataKeySalt returns the database's salt, from the cache or the
// database, or nil when it has none (no row, or no table).
func loadDataKeySalt(db *gorm.DB) ([]byte, error) {
	if salt, ok := dataKeySalts.Load(db.Config); ok {
		return salt.([]byte), nil
	}
	db = db.Session(&gorm.Session{NewDB: true})
	if !db.Migrator().HasTable(&DataKeySalt{}) {
		return nil, nil
	}
	var rows []DataKeySalt
	if err := db.Where("id = ?", dataKeySaltRowID).Limit(1).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("read the data key salt: %w", err)
	}
	if len(rows) == 0 || len(rows[0].Salt) == 0 {
		return nil, nil
	}
	dataKeySalts.Store(db.Config, rows[0].Salt)
	return rows[0].Salt, nil
}
