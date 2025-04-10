package models

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// MCPServer represents an MCP server configuration
type MCPServer struct {
	gorm.Model
	UserID      uint   `json:"user_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Endpoint    string `json:"endpoint"`
	Status      string `json:"status" gorm:"default:'stopped'"`
	LastStarted time.Time
	Tools       []Tool `gorm:"many2many:mcp_server_tools;"`
}

// MCPServers is a collection of MCPServer models
type MCPServers []MCPServer

// NewMCPServer creates a new MCP server configuration
func NewMCPServer() *MCPServer {
	return &MCPServer{
		Status: "stopped",
	}
}

// Create creates a new MCP server in the database
func (m *MCPServer) Create(db *gorm.DB) error {
	return db.Create(m).Error
}

// Update updates an existing MCP server in the database
func (m *MCPServer) Update(db *gorm.DB) error {
	return db.Save(m).Error
}

// Delete deletes an MCP server from the database
func (m *MCPServer) Delete(db *gorm.DB) error {
	return db.Delete(m).Error
}

// Get retrieves an MCP server by ID
func (m *MCPServer) Get(db *gorm.DB, id uint) error {
	return db.First(m, id).Error
}

// GetByUserID retrieves all MCP servers for a user
func (m *MCPServers) GetByUserID(db *gorm.DB, userID uint) error {
	return db.Where("user_id = ?", userID).Find(m).Error
}

// AddTool adds a tool to an MCP server
func (m *MCPServer) AddTool(db *gorm.DB, tool *Tool) error {
	return db.Model(m).Association("Tools").Append(tool)
}

// RemoveTool removes a tool from an MCP server
func (m *MCPServer) RemoveTool(db *gorm.DB, tool *Tool) error {
	return db.Model(m).Association("Tools").Delete(tool)
}

// GetTools gets all tools for an MCP server
func (m *MCPServer) GetTools(db *gorm.DB) ([]Tool, error) {
	var tools []Tool
	err := db.Model(m).Association("Tools").Find(&tools)
	return tools, err
}

// MarshalJSON implements the json.Marshaler interface
func (m *MCPServer) MarshalJSON() ([]byte, error) {
	type Alias MCPServer
	return json.Marshal(&struct {
		*Alias
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}{
		Alias:     (*Alias)(m),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	})
}
