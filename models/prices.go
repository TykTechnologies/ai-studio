package models

import "gorm.io/gorm"

type ModelPrice struct {
	gorm.Model

	ID        uint    `gorm:"primaryKey"`
	ModelName string  `json:"model_name"`
	Vendor    string  `json:"vendor"`
	CPT       float64 `json:"cpt"`
	Currency  string  `json:"currency"`
}

type ModelPrices []ModelPrice

// Create a new ModelPrice
func (mp *ModelPrice) Create(db *gorm.DB) error {
	return db.Create(mp).Error
}

// Get a ModelPrice by ID
func (mp *ModelPrice) Get(db *gorm.DB, id uint) error {
	return db.First(mp, id).Error
}

// Update an existing ModelPrice
func (mp *ModelPrice) Update(db *gorm.DB) error {
	return db.Save(mp).Error
}

// Delete a ModelPrice
func (mp *ModelPrice) Delete(db *gorm.DB) error {
	return db.Delete(mp).Error
}

// GetAll retrieves all ModelPrices
func (mps *ModelPrices) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&ModelPrice{})

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

	err := query.Find(mps).Error
	return totalCount, totalPages, err
}

// GetByVendor retrieves all ModelPrices for a specific vendor
func (mps *ModelPrices) GetByVendor(db *gorm.DB, vendor string) error {
	return db.Where("vendor = ?", vendor).Find(mps).Error
}

// GetByModelName retrieves a ModelPrice by its model name
func (mp *ModelPrice) GetByModelName(db *gorm.DB, modelName string) error {
	return db.Where("model_name = ?", modelName).First(mp).Error
}

// GetByModelNameAndVendor retrieves a ModelPrice by its model name and vendor
func (mp *ModelPrice) GetByModelNameAndVendor(db *gorm.DB, modelName string, vendor string) error {
	return db.Where("model_name = ? AND vendor = ?", modelName, vendor).First(mp).Error
}

// CreateMultiple creates multiple ModelPrices at once
func (mps *ModelPrices) CreateMultiple(db *gorm.DB) error {
	return db.Create(mps).Error
}

// UpdateMultiple updates multiple ModelPrices at once
func (mps *ModelPrices) UpdateMultiple(db *gorm.DB) error {
	for _, mp := range *mps {
		if err := db.Save(&mp).Error; err != nil {
			return err
		}
	}
	return nil
}

// DeleteMultiple deletes multiple ModelPrices at once
func (mps *ModelPrices) DeleteMultiple(db *gorm.DB) error {
	return db.Delete(mps).Error
}
