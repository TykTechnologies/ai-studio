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
	return db.First(g, id).Error
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

func (g *Groups) GetAll(db *gorm.DB) error {
	return db.Find(g).Error
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
	return db.Model(g).Association("DataCatalogues").Find(&g.DataCatalogues)
}

func (g *Group) AddToolCatalogue(db *gorm.DB, toolCatalogue *ToolCatalogue) error {
	return db.Model(g).Association("ToolCatalogues").Append(toolCatalogue)
}

func (g *Group) RemoveToolCatalogue(db *gorm.DB, toolCatalogue *ToolCatalogue) error {
	return db.Model(g).Association("ToolCatalogues").Delete(toolCatalogue)
}

func (g *Group) GetToolCatalogues(db *gorm.DB) error {
	return db.Model(g).Association("ToolCatalogues").Find(&g.ToolCatalogues)
}
