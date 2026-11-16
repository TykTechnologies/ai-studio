package models

import (
	"sync/atomic"
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models/wsstamp"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// BeforeUpdate on every stamped model (WorkspaceStampedModels): an update
// writes workspace_id only when it selects the column by name, so a struct
// rebuilt from a request (WorkspaceID 0) and saved cannot move the object
// (wsstamp.KeepOnUpdate). secrets.Secret has the same hook in its package.

func (l *LLM) BeforeUpdate(tx *gorm.DB) error            { return wsstamp.KeepOnUpdate(tx) }
func (a *App) BeforeUpdate(tx *gorm.DB) error            { return wsstamp.KeepOnUpdate(tx) }
func (t *Tool) BeforeUpdate(tx *gorm.DB) error           { return wsstamp.KeepOnUpdate(tx) }
func (d *Datasource) BeforeUpdate(tx *gorm.DB) error     { return wsstamp.KeepOnUpdate(tx) }
func (f *Filter) BeforeUpdate(tx *gorm.DB) error         { return wsstamp.KeepOnUpdate(tx) }
func (e *Embedder) BeforeUpdate(tx *gorm.DB) error       { return wsstamp.KeepOnUpdate(tx) }
func (m *ModelRouter) BeforeUpdate(tx *gorm.DB) error    { return wsstamp.KeepOnUpdate(tx) }
func (s *SemanticRouter) BeforeUpdate(tx *gorm.DB) error { return wsstamp.KeepOnUpdate(tx) }
func (a *AgentConfig) BeforeUpdate(tx *gorm.DB) error    { return wsstamp.KeepOnUpdate(tx) }
func (c *Catalogue) BeforeUpdate(tx *gorm.DB) error      { return wsstamp.KeepOnUpdate(tx) }
func (d *DataCatalogue) BeforeUpdate(tx *gorm.DB) error  { return wsstamp.KeepOnUpdate(tx) }
func (t *ToolCatalogue) BeforeUpdate(tx *gorm.DB) error  { return wsstamp.KeepOnUpdate(tx) }
func (f *FileStore) BeforeUpdate(tx *gorm.DB) error      { return wsstamp.KeepOnUpdate(tx) }
func (m *MCPServer) BeforeUpdate(tx *gorm.DB) error      { return wsstamp.KeepOnUpdate(tx) }
func (p *Plugin) BeforeUpdate(tx *gorm.DB) error         { return wsstamp.KeepOnUpdate(tx) }
func (w *WebhookTarget) BeforeUpdate(tx *gorm.DB) error  { return wsstamp.KeepOnUpdate(tx) }

// BeforeCreate on every stamped model (WorkspaceStampedModels): the Workspace
// from the statement's context when none is set, and the Workspace row locked
// for the insert (stampOnCreate). LLM's is in llm_resource_signing.go.
func (a *App) BeforeCreate(tx *gorm.DB) error            { return stampOnCreate(tx, &a.WorkspaceID) }
func (t *Tool) BeforeCreate(tx *gorm.DB) error           { return stampOnCreate(tx, &t.WorkspaceID) }
func (d *Datasource) BeforeCreate(tx *gorm.DB) error     { return stampOnCreate(tx, &d.WorkspaceID) }
func (f *Filter) BeforeCreate(tx *gorm.DB) error         { return stampOnCreate(tx, &f.WorkspaceID) }
func (e *Embedder) BeforeCreate(tx *gorm.DB) error       { return stampOnCreate(tx, &e.WorkspaceID) }
func (m *ModelRouter) BeforeCreate(tx *gorm.DB) error    { return stampOnCreate(tx, &m.WorkspaceID) }
func (s *SemanticRouter) BeforeCreate(tx *gorm.DB) error { return stampOnCreate(tx, &s.WorkspaceID) }
func (a *AgentConfig) BeforeCreate(tx *gorm.DB) error    { return stampOnCreate(tx, &a.WorkspaceID) }
func (c *Catalogue) BeforeCreate(tx *gorm.DB) error      { return stampOnCreate(tx, &c.WorkspaceID) }
func (d *DataCatalogue) BeforeCreate(tx *gorm.DB) error  { return stampOnCreate(tx, &d.WorkspaceID) }
func (t *ToolCatalogue) BeforeCreate(tx *gorm.DB) error  { return stampOnCreate(tx, &t.WorkspaceID) }
func (f *FileStore) BeforeCreate(tx *gorm.DB) error      { return stampOnCreate(tx, &f.WorkspaceID) }
func (m *MCPServer) BeforeCreate(tx *gorm.DB) error {
	return stampMCPServerOnCreate(tx, &m.WorkspaceID)
}
func (p *Plugin) BeforeCreate(tx *gorm.DB) error        { return stampOnCreate(tx, &p.WorkspaceID) }
func (w *WebhookTarget) BeforeCreate(tx *gorm.DB) error { return stampOnCreate(tx, &w.WorkspaceID) }

// mcpDefaultWorkspaceID caches the Default's id for stampMCPServerOnCreate.
var mcpDefaultWorkspaceID atomic.Uint64

// stampMCPServerOnCreate is stampOnCreate for MCP servers, which are also
// created with no acting user (Dashboard sync): one that would be written
// with 0 takes the Default workspace's id instead, so a catalogued server
// always names its Workspace (Workspaces B4b). Without a Default row (a
// test database without Workspaces) it stays 0, which reads as the Default.
func stampMCPServerOnCreate(tx *gorm.DB, workspaceID *uint) error {
	if err := stampOnCreate(tx, workspaceID); err != nil || *workspaceID != 0 {
		return err
	}
	if id := mcpDefaultWorkspaceID.Load(); id != 0 {
		*workspaceID = uint(id)
		return nil
	}
	var ids []uint
	if err := tx.Session(&gorm.Session{NewDB: true}).Table("workspaces").Where("is_default = ?", true).Limit(1).Pluck("id", &ids).Error; err != nil || len(ids) == 0 {
		return nil
	}
	*workspaceID = ids[0]
	if !testing.Testing() {
		// The Default can never be deleted or replaced, so its id is
		// read once per process (tests open many databases).
		mcpDefaultWorkspaceID.Store(uint64(ids[0]))
	}
	return nil
}
