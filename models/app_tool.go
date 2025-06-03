package models

// AppTool represents the join table for the many-to-many relationship
// between Apps and Tools.
type AppTool struct {
	AppID  uint `json:"app_id" gorm:"primaryKey"`
	ToolID uint `json:"tool_id" gorm:"primaryKey"`
}

// TableName specifies the custom table name for the AppTool model.
func (AppTool) TableName() string {
	return "app_tools"
}

// Ensure AppTool is registered with GORM for auto-migration
func init() {
	// This is a common pattern, but the actual registration might be centralized.
	// We'll verify this later.
	// Register(&AppTool{})
}

// Add any necessary methods for AppTool, if required.
// For a simple join table, methods might not be necessary.

// Example of how to add an App-Tool association (conceptual)
// func AddAppTool(db *gorm.DB, appID, toolID uint) error {
//  appTool := AppTool{AppID: appID, ToolID: toolID}
//  return db.Create(&appTool).Error
// }

// Example of how to remove an App-Tool association (conceptual)
// func RemoveAppTool(db *gorm.DB, appID, toolID uint) error {
//  return db.Where("app_id = ? AND tool_id = ?", appID, toolID).Delete(&AppTool{}).Error
// }

// Example of how to get tools for an app (conceptual)
// func GetToolsForApp(db *gorm.DB, appID uint) ([]Tool, error) {
//  var tools []Tool
//  err := db.Joins("JOIN app_tools ON app_tools.tool_id = tools.id").
//    Where("app_tools.app_id = ?", appID).Find(&tools).Error
//  return tools, err
// }

// Example of how to get apps for a tool (conceptual)
// func GetAppsForTool(db *gorm.DB, toolID uint) ([]App, error) {
//  var apps []App
//  err := db.Joins("JOIN app_tools ON app_tools.app_id = apps.id").
//    Where("app_tools.tool_id = ?", toolID).Find(&apps).Error
//  return apps, err
// }
