package models

import "gorm.io/gorm"

type Group struct {
	gorm.Model
	ID             uint            `json:"id" gorm:"primaryKey"`
	Name           string          `json:"name"`
	Users          []User          `json:"users" gorm:"many2many:user_groups;"`
	Catalogues     []Catalogue     `json:"catalogues" gorm:"many2many:group_catalogues;"`
	DataCatalogues []DataCatalogue `json:"data_catalogues" gorm:"many2many:group_datacatalogues;"`
	ToolCatalogues []ToolCatalogue `json:"tool_catalogues" gorm:"many2many:group_toolcatalogues;"`
}

type Groups []Group

func NewGroup() *Group {
	return &Group{}
}

func (g *Group) Get(db *gorm.DB, id uint) error {
	return db.Preload("Catalogues").
		Preload("DataCatalogues").
		Preload("ToolCatalogues").
		First(g, id).Error
}

func (gs *Groups) List(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Group{})

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

	err := query.Find(gs).Error
	return totalCount, totalPages, err
}

func (g *Group) Create(db *gorm.DB) error {
	return db.Create(g).Error
}

func (g *Group) Update(db *gorm.DB) error {
	return db.Save(g).Error
}

func (g *Group) Delete(db *gorm.DB) error {
	return db.Delete(g).Error
}

func (g *Group) AddUser(db *gorm.DB, user *User) error {
	return db.Model(g).Association("Users").Append(user)
}

func (g *Group) RemoveUser(db *gorm.DB, user *User) error {
	return db.Model(g).Association("Users").Delete(user)
}

func (g *Group) GetGroupUsers(db *gorm.DB) error {
	return db.Model(g).Association("Users").Find(&g.Users)
}

func (g *Groups) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Group{})

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

	err := query.Find(g).Error
	return totalCount, totalPages, err
}

func (g *Groups) GetByNameStub(db *gorm.DB, stub string) error {
	return db.Where("name LIKE ?", stub+"%").Find(g).Error
}

func (g *Group) AddCatalogue(db *gorm.DB, catalogue *Catalogue) error {
	return db.Model(g).Association("Catalogues").Append(catalogue)
}

func (g *Group) RemoveCatalogue(db *gorm.DB, catalogue *Catalogue) error {
	return db.Model(g).Association("Catalogues").Delete(catalogue)
}

func (g *Group) GetCatalogues(db *gorm.DB) error {
	return db.Model(g).Association("Catalogues").Find(&g.Catalogues)
}

func (g *Groups) GetGroupsByUserID(db *gorm.DB, userID uint) error {
	return db.Joins("JOIN user_groups ON user_groups.group_id = groups.id").
		Where("user_groups.user_id = ?", userID).
		Find(g).Error
}

func (g *Group) AddDataCatalogue(db *gorm.DB, dataCatalogue *DataCatalogue) error {
	return db.Model(g).Association("DataCatalogues").Append(dataCatalogue)
}

func (g *Group) RemoveDataCatalogue(db *gorm.DB, dataCatalogue *DataCatalogue) error {
	return db.Model(g).Association("DataCatalogues").Delete(dataCatalogue)
}

func (g *Group) GetDataCatalogues(db *gorm.DB) error {
	return db.Preload("Datasources", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, name") // Only select ID and Name fields from Datasources
	}).
		Model(g).
		Association("DataCatalogues").
		Find(&g.DataCatalogues)
}

func (g *Group) AddToolCatalogue(db *gorm.DB, toolCatalogue *ToolCatalogue) error {
	return db.Model(g).Association("ToolCatalogues").Append(toolCatalogue)
}

func (g *Group) RemoveToolCatalogue(db *gorm.DB, toolCatalogue *ToolCatalogue) error {
	return db.Model(g).Association("ToolCatalogues").Delete(toolCatalogue)
}

func (g *Group) GetToolCatalogues(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64

	// Count total number of ToolCatalogues
	countQuery := db.Table("tool_catalogues").
		Joins("JOIN group_toolcatalogues ON group_toolcatalogues.tool_catalogue_id = tool_catalogues.id").
		Where("group_toolcatalogues.group_id = ?", g.ID)

	if err := countQuery.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	// Calculate total pages
	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	// Base query
	query := db.Preload("Tools", func(db *gorm.DB) *gorm.DB {
		return db.Select("id, name") // Only select ID and Name fields from Tools
	}).
		Table("tool_catalogues").
		Select("tool_catalogues.*").
		Joins("JOIN group_toolcatalogues ON group_toolcatalogues.tool_catalogue_id = tool_catalogues.id").
		Where("group_toolcatalogues.group_id = ?", g.ID)

	if all {
		// Fetch all ToolCatalogues
		if err := query.Find(&g.ToolCatalogues).Error; err != nil {
			return 0, 0, err
		}
	} else {
		// Apply pagination
		offset := (pageNumber - 1) * pageSize
		if err := query.Offset(offset).Limit(pageSize).Find(&g.ToolCatalogues).Error; err != nil {
			return 0, 0, err
		}
	}

	return totalCount, totalPages, nil
}
