package models

import "gorm.io/gorm"

const DefaultGroupID uint = 1

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

func (g *Group) Get(db *gorm.DB, id uint, preloads ...string) error {
	query := db.Model(g)

	for _, preload := range preloads {
		query = query.Preload(preload)
	}

	return query.First(g, id).Error
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

func (g *Group) ReplaceAssociation(db *gorm.DB, associationName string, values interface{}) error {
	return db.Model(g).Association(associationName).Replace(values)
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

func (g *Groups) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool, sort string, preloads ...string) (int64, int, error) {
	query := db.Model(&Group{})

	for _, preload := range preloads {
		query = query.Preload(preload)
	}

	query, totalCount, totalPages, err := PaginateAndSort(query, pageSize, pageNumber, all, sort)
	if err != nil {
		return 0, 0, err
	}

	err = query.Find(g).Error
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

func (g *Group) ParseAssociations(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs []uint) {
	g.Users = make([]User, 0, len(userIDs))
	for _, userID := range userIDs {
		g.Users = append(g.Users, User{ID: userID})
	}

	g.Catalogues = make([]Catalogue, 0, len(catalogueIDs))
	for _, catalogueID := range catalogueIDs {
		g.Catalogues = append(g.Catalogues, Catalogue{ID: catalogueID})
	}

	g.DataCatalogues = make([]DataCatalogue, 0, len(dataCatalogueIDs))
	for _, dataCatalogueID := range dataCatalogueIDs {
		g.DataCatalogues = append(g.DataCatalogues, DataCatalogue{ID: dataCatalogueID})
	}

	g.ToolCatalogues = make([]ToolCatalogue, 0, len(toolCatalogueIDs))
	for _, toolCatalogueID := range toolCatalogueIDs {
		g.ToolCatalogues = append(g.ToolCatalogues, ToolCatalogue{ID: toolCatalogueID})
	}
}

func (g *Group) ExtractAssociationsIDs() (userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs []uint) {
	userIDs = make([]uint, len(g.Users))
	for i, user := range g.Users {
		userIDs[i] = user.ID
	}

	catalogueIDs = make([]uint, len(g.Catalogues))
	for i, catalogue := range g.Catalogues {
		catalogueIDs[i] = catalogue.ID
	}

	dataCatalogueIDs = make([]uint, len(g.DataCatalogues))
	for i, dataCatalogue := range g.DataCatalogues {
		dataCatalogueIDs[i] = dataCatalogue.ID
	}

	toolCatalogueIDs = make([]uint, len(g.ToolCatalogues))
	for i, toolCatalogue := range g.ToolCatalogues {
		toolCatalogueIDs[i] = toolCatalogue.ID
	}

	return
}

type AssociationData struct {
	Name        string
	NeedsUpdate bool
	GetValue    func() interface{}
}

func (g *Group) GetAssociationsToUpdate(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs []uint) []AssociationData {
	currentUserIDs, currentCatalogueIDs, currentDataCatalogueIDs, currentToolCatalogueIDs := g.ExtractAssociationsIDs()

	g.ParseAssociations(userIDs, catalogueIDs, dataCatalogueIDs, toolCatalogueIDs)

	return []AssociationData{
		{
			Name:        "Users",
			NeedsUpdate: !SameIDs(currentUserIDs, userIDs),
			GetValue:    func() interface{} { return g.Users },
		},
		{
			Name:        "Catalogues",
			NeedsUpdate: !SameIDs(currentCatalogueIDs, catalogueIDs),
			GetValue:    func() interface{} { return g.Catalogues },
		},
		{
			Name:        "DataCatalogues",
			NeedsUpdate: !SameIDs(currentDataCatalogueIDs, dataCatalogueIDs),
			GetValue:    func() interface{} { return g.DataCatalogues },
		},
		{
			Name:        "ToolCatalogues",
			NeedsUpdate: !SameIDs(currentToolCatalogueIDs, toolCatalogueIDs),
			GetValue:    func() interface{} { return g.ToolCatalogues },
		},
	}
}

func (g *Group) ClearAssociations(db *gorm.DB) error {
	if err := db.Model(g).Association("Users").Clear(); err != nil {
		return err
	}

	if err := db.Model(g).Association("Catalogues").Clear(); err != nil {
		return err
	}

	if err := db.Model(g).Association("DataCatalogues").Clear(); err != nil {
		return err
	}

	if err := db.Model(g).Association("ToolCatalogues").Clear(); err != nil {
		return err
	}

	return nil
}

func (g *Groups) SearchByTerm(db *gorm.DB, term string, pageSize int, pageNumber int, all bool, sort string, preloads ...string) (int64, int, error) {
	query := db.Model(&Group{})

	if term != "" {
		searchTerm := "%" + term + "%"
		query = query.Where("name LIKE ?", searchTerm)
	}

	for _, preload := range preloads {
		query = query.Preload(preload)
	}

	query, totalCount, totalPages, err := PaginateAndSort(query, pageSize, pageNumber, all, sort)
	if err != nil {
		return 0, 0, err
	}

	err = query.Find(g).Error
	return totalCount, totalPages, err
}

type GroupMemberCount struct {
	GroupID uint
	Count   int64
}

func (gs *Groups) GetGroupsMemberCounts(db *gorm.DB) ([]GroupMemberCount, error) {
	var results []GroupMemberCount

	groupIDs := make([]uint, len(*gs))
	for i, group := range *gs {
		groupIDs[i] = group.ID
	}

	err := db.Table("user_groups").
		Select("user_groups.group_id, COUNT(*) as count").
		Joins("JOIN users ON users.id = user_groups.user_id").
		Where("user_groups.group_id IN ? AND users.deleted_at IS NULL", groupIDs).
		Group("user_groups.group_id").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	return results, nil
}

func (g *Group) GetMembersCount(memberCounts []GroupMemberCount) int {
	for _, mc := range memberCounts {
		if mc.GroupID == g.ID {
			return int(mc.Count)
		}
	}

	return len(g.Users)
}

func (g *Group) GetCataloguesCount() int {
	return len(g.Catalogues)
}

func (g *Group) GetDataCataloguesCount() int {
	return len(g.DataCatalogues)
}

func (g *Group) GetToolCataloguesCount() int {
	return len(g.ToolCatalogues)
}

func IsGroupNameUnique(db *gorm.DB, name string, groupID uint) (bool, error) {
	var count int64
	query := db.Model(&Group{}).Where("name = ?", name)

	if groupID != 0 {
		query = query.Where("id != ?", groupID)
	}

	if err := query.Count(&count).Error; err != nil {
		return false, err
	}

	return count == 0, nil
}

func DefaultGroupExists(db *gorm.DB) (bool, error) {
	var count int64
	err := db.Model(&Group{}).Where("id = ?", DefaultGroupID).Count(&count).Error

	return count > 0, err
}

func ValidateGroupsExist(db *gorm.DB, groupIDs []uint) (bool, error) {
	if len(groupIDs) == 0 {
		return false, nil
	}

	var count int64
	err := db.Model(&Group{}).Where("id IN ?", groupIDs).Count(&count).Error
	if err != nil {
		return false, err
	}

	if count != int64(len(groupIDs)) {
		return false, nil
	}

	return true, nil
}
