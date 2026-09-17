package models

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gosimple/slug"
	"gorm.io/gorm"
)

type Tool struct {
	gorm.Model
	ID          uint   `json:"id" gorm:"primary_key"`
	Name        string `json:"name" gorm:"index"`
	Slug        string `json:"slug" gorm:"index"`
	Description string `json:"description"`

	ToolType            string `json:"tool_type"`
	OASSpec             string `json:"oas_spec"`
	AvailableOperations string `json:"available_operations"`
	PrivacyScore        int    `json:"privacy_score"`
	AuthKey             string `json:"auth_key"`
	AuthSchemaName      string `json:"auth_schema_name"`
	Active              bool   `json:"active" gorm:"default:true"`
	Namespace           string `json:"namespace" gorm:"default:'';index:idx_tool_namespace"`

	// Access methods. A tool is first of all a chat capability; reaching it
	// from an App over REST or MCP on the gateway is optional and switched per
	// tool. The columns are stored inverted so the zero value means "enabled":
	// rows that predate the switches, and config pushed by a hub that does not
	// know them, keep today's behaviour. New tools get their default in
	// services.CreateToolWithDB (see DefaultToolRESTAccessEnabled). Read them
	// through RESTAccessEnabled / MCPAccessEnabled, never directly.
	RESTAccessDisabled bool `json:"rest_access_disabled" gorm:"not null;default:false"`
	MCPAccessDisabled  bool `json:"mcp_access_disabled" gorm:"not null;default:false"`

	FileStores  []FileStore `gorm:"many2many:tool_filestores;" json:"file_stores"`
	Filters      []Filter    `gorm:"many2many:tool_filters;" json:"filters"`
	Dependencies []*Tool     `gorm:"many2many:tool_dependencies" json:"dependencies"`
	Apps         []*App      `gorm:"many2many:app_tools;" json:"apps"`

	// Ownership
	UserID uint `json:"user_id" gorm:"index:idx_tool_user_community"`

	// UGC (User-Generated Content) fields
	CommunitySubmitted bool  `json:"community_submitted" gorm:"index:idx_tool_user_community"`
	SubmissionID       *uint `json:"submission_id"`

	// Plugin-stored metadata
	Metadata JSONMap `json:"metadata" gorm:"type:json"`
}

type Tools []Tool

const (
	ToolTypeREST = "REST"
	// ToolTypeClient is a human-in-the-loop tool: the model calls it like any
	// function, but it is executed by the person in the chat UI (a form or an
	// approval) rather than by the server. Its definition lives in OASSpec as
	// a ClientToolDefinition JSON document.
	ToolTypeClient = "CLIENT"
)

// Defaults for the access methods of a newly created tool. Tools that existed
// before the switches stay enabled (see Tool.RESTAccessDisabled); a new tool is
// chat only until an admin turns a method on.
const (
	DefaultToolRESTAccessEnabled = false
	DefaultToolMCPAccessEnabled  = false
)

const (
	// ClientToolKindApproval shows Approve / Reject buttons.
	ClientToolKindApproval = "approval"
	// ClientToolKindForm shows a JSON-Schema form (ResponseSchema).
	ClientToolKindForm = "form"
	// ClientToolKindPresent is generative UI: the model composes cards,
	// facts, tables, charts and forms from the built-in component
	// vocabulary (see PresentToolSchema) and the chat draws them. The
	// call resolves in the browser without user input.
	ClientToolKindPresent = "present"
)

// ClientToolUI tells the chat UI how to collect the tool's result.
type ClientToolUI struct {
	// Kind is "form" (the user fills ResponseSchema), "approval" (the user
	// approves or rejects what the model asked for) or "present"
	// (generative UI drawn from the built-in vocabulary).
	Kind string `json:"kind"`
	// ResponseSchema is the JSON schema of the value the user provides for
	// kind "form". Optional; a single free-text field is used when absent.
	ResponseSchema map[string]interface{} `json:"response_schema,omitempty"`
	// Title and Description label the card shown to the user.
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// ClientToolDefinition is the OASSpec payload of a ToolTypeClient tool.
type ClientToolDefinition struct {
	// Parameters is the JSON schema of the arguments the model supplies.
	Parameters map[string]interface{} `json:"parameters"`
	UI         ClientToolUI           `json:"ui"`
}

// ClientDefinition parses the tool's client-tool definition. Missing pieces
// get usable defaults: an empty object schema and an approval UI.
func (t *Tool) ClientDefinition() (*ClientToolDefinition, error) {
	def := &ClientToolDefinition{}
	if raw := strings.TrimSpace(t.OASSpec); raw != "" {
		if err := json.Unmarshal([]byte(raw), def); err != nil {
			// The API stores specs base64-encoded; accept that form too.
			decoded, decErr := base64.StdEncoding.DecodeString(raw)
			if decErr != nil || json.Unmarshal(decoded, def) != nil {
				return nil, fmt.Errorf("invalid client tool definition: %w", err)
			}
		}
	}
	if def.UI.Kind == "" {
		def.UI.Kind = ClientToolKindApproval
	}
	if def.UI.Kind == ClientToolKindPresent && len(def.Parameters) == 0 {
		// The vocabulary schema is built in; admins do not author it.
		spec, err := PresentToolSchema()
		if err != nil {
			return nil, fmt.Errorf("generative UI schema: %w", err)
		}
		def.Parameters = spec.Parameters
		if def.UI.Description == "" {
			def.UI.Description = spec.Description
		}
	}
	if def.Parameters == nil {
		def.Parameters = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
	}
	return def, nil
}

// ClientOperation is the function name the model sees for a client tool:
// the first configured operation, else the tool's slug.
func (t *Tool) ClientOperation() string {
	if ops := t.GetOperations(); len(ops) > 0 && strings.TrimSpace(ops[0]) != "" {
		return strings.TrimSpace(ops[0])
	}
	if t.Slug != "" {
		return t.Slug
	}
	return slug.Make(t.Name)
}

func NewTool() *Tool {
	return &Tool{}
}

// BeforeSave computes the slug from the tool name before saving
func (t *Tool) BeforeSave(tx *gorm.DB) error {
	t.Slug = slug.Make(t.Name)
	return nil
}

// Create a new tool
func (t *Tool) Create(db *gorm.DB) error {
	return db.Create(t).Error
}

// Get a tool by ID
func (t *Tool) Get(db *gorm.DB, id uint) error {
	return db.Preload("FileStores").Preload("Filters").Preload("Dependencies").Preload("Apps").First(t, id).Error
}

// Update an existing tool
func (t *Tool) Update(db *gorm.DB) error {
	return db.Save(t).Error
}

// Delete a tool
func (t *Tool) Delete(db *gorm.DB) error {
	return db.Delete(t).Error
}

// GetByName gets a tool by its name
func (t *Tool) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).Preload("FileStores").Preload("Filters").Preload("Dependencies").Preload("Apps").First(t).Error
}

// GetAll retrieves all tools
func (t *Tools) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) (int64, int, error) {
	var totalCount int64
	query := db.Model(&Tool{})

	// Optional search/sort scopes (services.ListOptions); applied before the
	// count so X-Total-Count reflects the filtered set.
	for _, scope := range scopes {
		query = scope(query)
	}
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

	err := query.Preload("Apps").Find(t).Error
	return totalCount, totalPages, err
}

// GetByType retrieves all tools of a specific type
func (t *Tools) GetByType(db *gorm.DB, toolType string) error {
	return db.Where("tool_type = ?", toolType).Preload("Apps").Find(t).Error
}

// GetByPrivacyScoreMin retrieves all tools with a privacy score greater than or equal to the given minimum
func (t *Tools) GetByPrivacyScoreMin(db *gorm.DB, minScore int) error {
	return db.Where("privacy_score >= ?", minScore).Preload("Apps").Find(t).Error
}

// GetByPrivacyScoreMax retrieves all tools with a privacy score less than or equal to the given maximum
func (t *Tools) GetByPrivacyScoreMax(db *gorm.DB, maxScore int) error {
	return db.Where("privacy_score <= ?", maxScore).Preload("Apps").Find(t).Error
}

// GetByPrivacyScoreRange retrieves all tools with a privacy score within the given range
func (t *Tools) GetByPrivacyScoreRange(db *gorm.DB, minScore, maxScore int) error {
	return db.Where("privacy_score BETWEEN ? AND ?", minScore, maxScore).Preload("Apps").Find(t).Error
}

// Search retrieves all tools matching the given query in name or description
func (t *Tools) Search(db *gorm.DB, query string) error {
	return db.Where("name LIKE ? OR description LIKE ?", "%"+query+"%", "%"+query+"%").Preload("Apps").Find(t).Error
}

// AddOperation adds a new operation to the AvailableOperations list
func (t *Tool) AddOperation(operation string) {
	operations := t.GetOperations()
	for _, op := range operations {
		if op == operation {
			return // Operation already exists, do nothing
		}
	}
	if t.AvailableOperations == "" {
		t.AvailableOperations = operation
	} else {
		t.AvailableOperations += "," + operation
	}
}

// RemoveOperation removes an operation from the AvailableOperations list
func (t *Tool) RemoveOperation(operation string) {
	operations := t.GetOperations()
	var newOperations []string
	for _, op := range operations {
		if op != operation {
			newOperations = append(newOperations, op)
		}
	}
	t.AvailableOperations = strings.Join(newOperations, ",")
}

// GetOperations returns the AvailableOperations as a []string
func (t *Tool) GetOperations() []string {
	if t.AvailableOperations == "" {
		return []string{}
	}
	return strings.Split(t.AvailableOperations, ",")
}

// AllowsOperation reports whether operationID is on the tool's whitelist. An
// empty whitelist allows nothing, which is what chat and the MCP endpoint
// already do.
func (t *Tool) AllowsOperation(operationID string) bool {
	if operationID == "" {
		return false
	}
	for _, op := range t.GetOperations() {
		if strings.TrimSpace(op) == operationID {
			return true
		}
	}
	return false
}

// servedByGateway reports whether the gateway can serve the tool at all. A
// client tool runs in the chat UI and has no gateway endpoint.
func (t *Tool) servedByGateway() bool {
	return t.ToolType != ToolTypeClient
}

// RESTAccessEnabled reports whether Apps may call the tool on /tools/{slug}.
func (t *Tool) RESTAccessEnabled() bool {
	return t.servedByGateway() && !t.RESTAccessDisabled
}

// MCPAccessEnabled reports whether Apps may reach the tool's MCP endpoint
// (/tools/{slug}/mcp and its SSE transport).
func (t *Tool) MCPAccessEnabled() bool {
	return t.servedByGateway() && !t.MCPAccessDisabled
}

// AppGrantable reports whether binding the tool to an App gives the App
// anything: a chat-only tool has no endpoint an App credential could unlock.
func (t *Tool) AppGrantable() bool {
	return t.RESTAccessEnabled() || t.MCPAccessEnabled()
}

// AddFileStore adds a FileStore to the Tool
func (t *Tool) AddFileStore(db *gorm.DB, fileStore *FileStore) error {
	return db.Model(t).Association("FileStores").Append(fileStore)
}

// RemoveFileStore removes a FileStore from the Tool
func (t *Tool) RemoveFileStore(db *gorm.DB, fileStore *FileStore) error {
	return db.Model(t).Association("FileStores").Delete(fileStore)
}

// GetFileStores gets all FileStores associated with the Tool
func (t *Tool) GetFileStores(db *gorm.DB) ([]FileStore, error) {
	var fileStores []FileStore
	err := db.Model(t).Association("FileStores").Find(&fileStores)
	return fileStores, err
}

// SetFileStores replaces all existing FileStore associations with new ones
func (t *Tool) SetFileStores(db *gorm.DB, fileStores []FileStore) error {
	return db.Model(t).Association("FileStores").Replace(&fileStores)
}

// AddFilter adds a Filter to the Tool
func (t *Tool) AddFilter(db *gorm.DB, filter *Filter) error {
	return db.Model(t).Association("Filters").Append(filter)
}

// RemoveFilter removes a Filter from the Tool
func (t *Tool) RemoveFilter(db *gorm.DB, filter *Filter) error {
	return db.Model(t).Association("Filters").Delete(filter)
}

// GetFilters gets all Filters associated with the Tool
func (t *Tool) GetFilters(db *gorm.DB) ([]Filter, error) {
	var filters []Filter
	err := db.Model(t).Association("Filters").Find(&filters)
	return filters, err
}

// SetFilters replaces all existing Filter associations with new ones
func (t *Tool) SetFilters(db *gorm.DB, filters []Filter) error {
	return db.Model(t).Association("Filters").Replace(&filters)
}

func (t *Tool) AddDependency(db *gorm.DB, dependency *Tool) error {
	// Prevent self-dependency
	if t.ID == dependency.ID {
		return fmt.Errorf("tool cannot depend on itself")
	}

	// Check for circular dependencies
	isCircular, err := t.WouldCreateCircularDependency(db, dependency)
	if err != nil {
		return err
	}
	if isCircular {
		return fmt.Errorf("adding this dependency would create a circular reference")
	}

	return db.Model(t).Association("Dependencies").Append(dependency)
}

// RemoveDependency removes a Tool dependency
func (t *Tool) RemoveDependency(db *gorm.DB, dependency *Tool) error {
	return db.Model(t).Association("Dependencies").Delete(dependency)
}

// GetDependencies gets all Tool dependencies
func (t *Tool) GetDependencies(db *gorm.DB) ([]*Tool, error) {
	var dependencies []*Tool
	err := db.Model(t).Association("Dependencies").Find(&dependencies)
	return dependencies, err
}

// SetDependencies replaces all existing Tool dependencies with new ones
func (t *Tool) SetDependencies(db *gorm.DB, dependencies []*Tool) error {
	return db.Model(t).Association("Dependencies").Replace(dependencies)
}

// ClearDependencies removes all Tool dependencies
func (t *Tool) ClearDependencies(db *gorm.DB) error {
	return db.Model(t).Association("Dependencies").Clear()
}

// HasDependency checks if a specific Tool is a dependency
func (t *Tool) HasDependency(db *gorm.DB, dependencyID uint) (bool, error) {
	var count int64
	err := db.Model(t).Where("id = ?", t.ID).
		Joins("JOIN tool_dependencies ON tool_dependencies.tool_id = tools.id").
		Where("tool_dependencies.dependency_id = ?", dependencyID).
		Count(&count).Error
	return count > 0, err
}

// Would create a circular dependency checks if adding this dependency would create a circular reference
func (t *Tool) WouldCreateCircularDependency(db *gorm.DB, newDependency *Tool) (bool, error) {
	// First check if the new dependency depends on the current tool directly
	var count int64
	err := db.Model(newDependency).
		Joins("JOIN tool_dependencies ON tool_dependencies.tool_id = tools.id").
		Where("tool_dependencies.dependency_id = ?", t.ID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	// Then check recursively through all dependencies of the new dependency
	dependencies, err := newDependency.GetDependencies(db)
	if err != nil {
		return false, err
	}

	for _, dep := range dependencies {
		// Check if this dependency is the current tool (would create a cycle)
		if dep.ID == t.ID {
			return true, nil
		}

		// Recursively check this dependency's dependencies
		isCircular, err := t.WouldCreateCircularDependency(db, dep)
		if err != nil {
			return false, err
		}
		if isCircular {
			return true, nil
		}
	}

	return false, nil
}
