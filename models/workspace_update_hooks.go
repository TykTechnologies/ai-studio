package models

import (
	"github.com/TykTechnologies/midsommar/v2/models/wsstamp"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// BeforeUpdate on every stamped model (WorkspaceStampedModels): an update
// writes workspace_id only when it selects the column by name, so a struct
// rebuilt from a request (WorkspaceID 0) and saved cannot move the object
// (wsstamp.KeepOnUpdate). secrets.Secret has the same hook in its package.

func (l *LLM) BeforeUpdate(tx *gorm.DB) error            { wsstamp.KeepOnUpdate(tx); return nil }
func (a *App) BeforeUpdate(tx *gorm.DB) error            { wsstamp.KeepOnUpdate(tx); return nil }
func (t *Tool) BeforeUpdate(tx *gorm.DB) error           { wsstamp.KeepOnUpdate(tx); return nil }
func (d *Datasource) BeforeUpdate(tx *gorm.DB) error     { wsstamp.KeepOnUpdate(tx); return nil }
func (f *Filter) BeforeUpdate(tx *gorm.DB) error         { wsstamp.KeepOnUpdate(tx); return nil }
func (e *Embedder) BeforeUpdate(tx *gorm.DB) error       { wsstamp.KeepOnUpdate(tx); return nil }
func (m *ModelRouter) BeforeUpdate(tx *gorm.DB) error    { wsstamp.KeepOnUpdate(tx); return nil }
func (s *SemanticRouter) BeforeUpdate(tx *gorm.DB) error { wsstamp.KeepOnUpdate(tx); return nil }
func (a *AgentConfig) BeforeUpdate(tx *gorm.DB) error    { wsstamp.KeepOnUpdate(tx); return nil }
func (c *Catalogue) BeforeUpdate(tx *gorm.DB) error      { wsstamp.KeepOnUpdate(tx); return nil }
func (d *DataCatalogue) BeforeUpdate(tx *gorm.DB) error  { wsstamp.KeepOnUpdate(tx); return nil }
func (t *ToolCatalogue) BeforeUpdate(tx *gorm.DB) error  { wsstamp.KeepOnUpdate(tx); return nil }
func (f *FileStore) BeforeUpdate(tx *gorm.DB) error      { wsstamp.KeepOnUpdate(tx); return nil }
func (m *MCPServer) BeforeUpdate(tx *gorm.DB) error      { wsstamp.KeepOnUpdate(tx); return nil }
func (p *Plugin) BeforeUpdate(tx *gorm.DB) error         { wsstamp.KeepOnUpdate(tx); return nil }
func (w *WebhookTarget) BeforeUpdate(tx *gorm.DB) error  { wsstamp.KeepOnUpdate(tx); return nil }
