package models

import (
	"time"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Kinds of gateway endpoint that carry an auth plugin list. LLMs are not
// here: their plugins (of every hook type, auth among them) live in
// llm_plugins.
const (
	EndpointTypeDatasource     = "datasource"
	EndpointTypeTool           = "tool"
	EndpointTypeModelRouter    = "model_router"
	EndpointTypeSemanticRouter = "semantic_router"
	EndpointTypePlugin         = "plugin" // a custom_endpoint plugin's /plugins/{slug}/ routes
)

// EndpointTypes lists the endpoint kinds, in display order.
var EndpointTypes = []string{
	EndpointTypeDatasource,
	EndpointTypeTool,
	EndpointTypeModelRouter,
	EndpointTypeSemanticRouter,
	EndpointTypePlugin,
}

// IsEndpointType reports whether t is one of EndpointTypes.
func IsEndpointType(t string) bool {
	for _, k := range EndpointTypes {
		if k == t {
			return true
		}
	}
	return false
}

// EndpointAuthPlugin attaches an auth plugin to a gateway endpoint other than
// an LLM. When an endpoint has any, they alone authenticate its requests, in
// OrderIndex order, as an LLM's attached auth plugins do.
//
// The list lives in its own table rather than on the Datasource, Tool or
// router structs so the full-replace updates those objects get (the REST
// forms, the plugin SDK) cannot clear it, and their RPC shapes stay as they
// are. Rows are hard-deleted: there is no history to keep, and a tombstone
// would collide with the primary key when the same plugin is attached again.
type EndpointAuthPlugin struct {
	ObjectType string    `json:"object_type" gorm:"primaryKey;size:32;index:idx_endpoint_auth_plugins_order,priority:1"`
	ObjectID   uint      `json:"object_id" gorm:"primaryKey;autoIncrement:false;index:idx_endpoint_auth_plugins_order,priority:2"`
	PluginID   uint      `json:"plugin_id" gorm:"primaryKey;autoIncrement:false;index:idx_endpoint_auth_plugins_plugin"`
	OrderIndex int       `json:"order_index" gorm:"default:0;index:idx_endpoint_auth_plugins_order,priority:3"`
	CreatedAt  time.Time `json:"created_at"`
}

// GetEndpointAuthPluginIDs returns the auth plugins attached to an endpoint,
// in order.
func GetEndpointAuthPluginIDs(db *gorm.DB, objectType string, objectID uint) ([]uint, error) {
	var ids []uint
	err := db.Model(&EndpointAuthPlugin{}).
		Where("object_type = ? AND object_id = ?", objectType, objectID).
		Order("order_index ASC").
		Pluck("plugin_id", &ids).Error
	return ids, err
}

// GetAllEndpointAuthPlugins returns every attachment, grouped by endpoint and
// ordered within it. The configuration snapshot reads them in one query.
func GetAllEndpointAuthPlugins(db *gorm.DB) ([]EndpointAuthPlugin, error) {
	var rows []EndpointAuthPlugin
	err := db.Order("object_type ASC, object_id ASC, order_index ASC").Find(&rows).Error
	return rows, err
}

// SetEndpointAuthPlugins replaces an endpoint's auth plugin list; the slice
// order is the execution order. An empty list detaches them all.
func SetEndpointAuthPlugins(db *gorm.DB, objectType string, objectID uint, pluginIDs []uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := DeleteEndpointAuthPlugins(tx, objectType, objectID); err != nil {
			return err
		}
		now := time.Now()
		for i, pluginID := range pluginIDs {
			row := EndpointAuthPlugin{
				ObjectType: objectType,
				ObjectID:   objectID,
				PluginID:   pluginID,
				OrderIndex: i,
				CreatedAt:  now,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteEndpointAuthPlugins detaches every auth plugin from an endpoint. The
// endpoint's delete calls it.
func DeleteEndpointAuthPlugins(db *gorm.DB, objectType string, objectID uint) error {
	return db.Where("object_type = ? AND object_id = ?", objectType, objectID).
		Delete(&EndpointAuthPlugin{}).Error
}

// DeleteEndpointAuthPluginsForPlugin detaches a plugin from every endpoint,
// and removes the list of a custom_endpoint plugin itself. The plugin's
// delete calls it.
func DeleteEndpointAuthPluginsForPlugin(db *gorm.DB, pluginID uint) error {
	return db.Where("plugin_id = ? OR (object_type = ? AND object_id = ?)", pluginID, EndpointTypePlugin, pluginID).
		Delete(&EndpointAuthPlugin{}).Error
}
