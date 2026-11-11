package models

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm/clause"
)

// The branding assets, by BrandingAsset.Kind.
const (
	BrandingAssetLogo    = "logo"
	BrandingAssetFavicon = "favicon"
)

// BrandingAsset is an uploaded logo or favicon. The bytes are in the
// database, not on the disk of the replica that received the upload, so
// every replica (in every site) serves the same asset. One row per kind.
type BrandingAsset struct {
	Kind        string `json:"kind" gorm:"primaryKey;size:16"`
	ContentType string `json:"content_type" gorm:"size:64;not null"`
	// Data is the file as uploaded (a logo up to 2 MB, a favicon up to 100 KB).
	Data []byte `json:"-" gorm:"not null"`
	// Hash is the SHA-256 of Data in hex: the ETag, and how a replica
	// checks that the copy it holds in memory is current.
	Hash      string    `json:"hash" gorm:"size:64;not null"`
	Size      int64     `json:"size" gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SaveBrandingAsset stores asset in place of any asset of its kind.
func SaveBrandingAsset(db *gorm.DB, asset *BrandingAsset) error {
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "kind"}},
		DoUpdates: clause.AssignmentColumns([]string{"content_type", "data", "hash", "size", "updated_at"}),
	}).Create(asset).Error
}

// AddBrandingAsset stores asset unless one of its kind exists, and reports
// whether it did.
func AddBrandingAsset(db *gorm.DB, asset *BrandingAsset) (bool, error) {
	res := db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "kind"}}, DoNothing: true}).Create(asset)
	return res.Error == nil && res.RowsAffected > 0, res.Error
}

// GetBrandingAsset returns the stored asset of kind, or nil when there is
// none.
func GetBrandingAsset(db *gorm.DB, kind string) (*BrandingAsset, error) {
	var assets []BrandingAsset
	if err := db.Where("kind = ?", kind).Limit(1).Find(&assets).Error; err != nil {
		return nil, err
	}
	if len(assets) == 0 {
		return nil, nil
	}
	return &assets[0], nil
}

// BrandingAssetHash returns the hash of the stored asset of kind, or ""
// when there is none, without reading its bytes.
func BrandingAssetHash(db *gorm.DB, kind string) (string, error) {
	var hashes []string
	if err := db.Model(&BrandingAsset{}).Where("kind = ?", kind).Limit(1).Pluck("hash", &hashes).Error; err != nil {
		return "", err
	}
	if len(hashes) == 0 {
		return "", nil
	}
	return hashes[0], nil
}

// DeleteBrandingAssets removes every stored asset.
func DeleteBrandingAssets(db *gorm.DB) error {
	return db.Where("1 = 1").Delete(&BrandingAsset{}).Error
}
