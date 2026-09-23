package database

import (
	"sync/atomic"

	"gorm.io/gorm"
)

// Config generation.
//
// The gateway reads the same configuration (LLMs, plugins, apps and their
// links) on every request. Caches of that configuration record the generation
// they were filled at and treat a changed generation as a miss. The generation
// is bumped by a GORM callback after every write, so a cache cannot go stale
// because some write path forgot to invalidate it: the control-plane sync, the
// reload handler, the management API and pull-on-miss all write through GORM.
//
// Only writes to runtimeTables leave the generation alone. They change on
// every request (analytics, budget usage) or hold state no cache reads, and
// bumping on them would empty the caches constantly. Any other table,
// including one added later or a raw statement whose table GORM cannot name,
// invalidates, so an omission here costs performance, never correctness.

var configGeneration atomic.Uint64

// runtimeTables are written as a side effect of serving traffic and are never
// read through a generation-checked cache.
var runtimeTables = map[string]bool{
	"analytics_events": true,
	"budget_usage":     true,
	"plugin_kv":        true,
	"edge_instances":   true,
	"control_payloads": true,
	"sync_states":      true,
	"token_cache":      true,
	"access_tokens":    true,
}

// ConfigGeneration returns the current configuration generation.
func ConfigGeneration() uint64 { return configGeneration.Load() }

// BumpConfigGeneration invalidates every generation-checked cache.
func BumpConfigGeneration() { configGeneration.Add(1) }

const configGenerationCallback = "mgw:config_generation"

// EnsureConfigGenerationCallbacks registers the generation callbacks on db if
// they are not there yet. Connect registers them; components that keep a
// generation-checked cache call it too, so the cache is invalidated even for
// a *gorm.DB opened some other way (tests, tools).
func EnsureConfigGenerationCallbacks(db *gorm.DB) error {
	if db == nil || db.Callback().Create().Get(configGenerationCallback) != nil {
		return nil
	}
	return registerConfigGenerationCallbacks(db)
}

// registerConfigGenerationCallbacks bumps the generation after each create,
// update, delete and raw statement on a configuration table.
func registerConfigGenerationCallbacks(db *gorm.DB) error {
	bump := func(tx *gorm.DB) {
		if runtimeTables[tx.Statement.Table] {
			return
		}
		BumpConfigGeneration()
	}
	cb := db.Callback()
	if err := cb.Create().After("gorm:create").Register(configGenerationCallback, bump); err != nil {
		return err
	}
	if err := cb.Update().After("gorm:update").Register(configGenerationCallback, bump); err != nil {
		return err
	}
	if err := cb.Delete().After("gorm:delete").Register(configGenerationCallback, bump); err != nil {
		return err
	}
	return cb.Raw().After("gorm:raw").Register(configGenerationCallback, bump)
}
