package models

import (
	"gorm.io/gorm"
)

// SelectionAlgorithm defines how vendors are selected within a pool
type SelectionAlgorithm string

const (
	SelectionRoundRobin SelectionAlgorithm = "round_robin"
	SelectionWeighted   SelectionAlgorithm = "weighted"
)

// APICompatibility defines the API format the router accepts
type APICompatibility string

const (
	APICompatOpenAI APICompatibility = "openai"
)

// ModelRouter is the top-level entity that defines a routing endpoint
// Routes are exposed at /router/{slug}/* and route requests to LLM vendors
// based on model name pattern matching
type ModelRouter struct {
	gorm.Model
	ID          uint         `json:"id" gorm:"primaryKey"`
	Name        string       `json:"name" gorm:"not null"`
	Slug        string       `json:"slug" gorm:"uniqueIndex:idx_router_slug_namespace;not null"`
	Description string       `json:"description"`
	APICompat   string       `json:"api_compat" gorm:"default:'openai'"` // Currently only 'openai' supported
	Active      bool         `json:"active" gorm:"default:false"`
	Namespace   string       `json:"namespace" gorm:"default:'';uniqueIndex:idx_router_slug_namespace;index:idx_router_namespace"`
	Pools       []*ModelPool `json:"pools" gorm:"foreignKey:RouterID;constraint:OnDelete:CASCADE"`
}

type ModelRouters []ModelRouter

// ModelPool groups vendors that handle specific model patterns
// Uses glob patterns (e.g., "claude-*", "gpt-4*") to match incoming model names
type ModelPool struct {
	gorm.Model
	ID                 uint               `json:"id" gorm:"primaryKey"`
	RouterID           uint               `json:"router_id" gorm:"not null;index:idx_pool_router"`
	Name               string             `json:"name" gorm:"not null"`
	ModelPattern       string             `json:"model_pattern" gorm:"not null"` // Glob pattern, e.g., "claude-*"
	SelectionAlgorithm SelectionAlgorithm `json:"selection_algorithm" gorm:"default:'round_robin'"`
	Priority           int                `json:"priority" gorm:"default:0"` // Higher priority pools are checked first
	Vendors            []*PoolVendor      `json:"vendors" gorm:"foreignKey:PoolID;constraint:OnDelete:CASCADE"`
}

type ModelPools []ModelPool

// PoolVendor represents an LLM vendor within a pool
// Weight is used for weighted selection algorithm
type PoolVendor struct {
	gorm.Model
	ID       uint            `json:"id" gorm:"primaryKey"`
	PoolID   uint            `json:"pool_id" gorm:"not null;index:idx_vendor_pool"`
	LLMID    uint            `json:"llm_id" gorm:"not null;index:idx_vendor_llm"`
	Weight   int             `json:"weight" gorm:"default:1"` // Used for weighted selection
	Active   bool            `json:"active" gorm:"default:true"`
	LLM      *LLM            `json:"llm,omitempty" gorm:"foreignKey:LLMID"`
	Mappings []*ModelMapping `json:"mappings" gorm:"foreignKey:VendorID;constraint:OnDelete:CASCADE"`
}

type PoolVendors []PoolVendor

// ModelMapping allows renaming models for a specific vendor
// e.g., map "gpt-4" to "claude-3-opus" when routing to Anthropic vendor
type ModelMapping struct {
	gorm.Model
	ID          uint   `json:"id" gorm:"primaryKey"`
	VendorID    uint   `json:"vendor_id" gorm:"not null;index:idx_mapping_vendor"`
	SourceModel string `json:"source_model" gorm:"not null"` // Model name from request
	TargetModel string `json:"target_model" gorm:"not null"` // Model name to send to this vendor
}

type ModelMappings []ModelMapping

// NewModelRouter creates a new ModelRouter instance
func NewModelRouter() *ModelRouter {
	return &ModelRouter{}
}

// Get retrieves a ModelRouter by ID with all relationships
func (r *ModelRouter) Get(db *gorm.DB, id uint) error {
	return db.Preload("Pools.Vendors.LLM").Preload("Pools.Vendors.Mappings").First(r, id).Error
}

// GetBySlug retrieves a ModelRouter by slug within a namespace
func (r *ModelRouter) GetBySlug(db *gorm.DB, slug string, namespace string) error {
	return db.Preload("Pools.Vendors.LLM").Preload("Pools.Vendors.Mappings").
		Where("slug = ? AND namespace = ?", slug, namespace).First(r).Error
}

// Create creates a new ModelRouter with all nested relationships
func (r *ModelRouter) Create(db *gorm.DB) error {
	tx := db.Begin()
	defer func() {
		if rec := recover(); rec != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Error; err != nil {
		return err
	}

	if err := tx.Create(r).Error; err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit().Error
}

// Update updates a ModelRouter and its relationships
func (r *ModelRouter) Update(db *gorm.DB) error {
	tx := db.Begin()
	defer func() {
		if rec := recover(); rec != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Error; err != nil {
		return err
	}

	// Save the main router
	if err := tx.Save(r).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Handle pools - delete existing and recreate
	// This ensures proper cascade handling of nested vendors and mappings
	if err := tx.Where("router_id = ?", r.ID).Delete(&ModelPool{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Recreate pools with their nested relationships
	// Clear IDs to ensure INSERT rather than UPDATE (pools were deleted above)
	for _, pool := range r.Pools {
		pool.ID = 0 // Clear pool ID to force INSERT
		pool.RouterID = r.ID
		// Clear nested vendor IDs and their mapping IDs
		for i := range pool.Vendors {
			pool.Vendors[i].ID = 0
			pool.Vendors[i].PoolID = 0 // Will be set by GORM after pool is created
			// Clear nested mapping IDs for this vendor
			for j := range pool.Vendors[i].Mappings {
				pool.Vendors[i].Mappings[j].ID = 0
				pool.Vendors[i].Mappings[j].VendorID = 0 // Will be set by GORM after vendor is created
			}
		}
		if err := tx.Create(pool).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

// Delete removes a ModelRouter (cascades to pools, vendors, mappings)
func (r *ModelRouter) Delete(db *gorm.DB) error {
	return db.Delete(r).Error
}

// GetAll retrieves all ModelRouters with pagination
func (r *ModelRouters) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool) (int64, int, error) {
	var totalCount int64
	query := db.Model(&ModelRouter{}).Preload("Pools.Vendors.LLM").Preload("Pools.Vendors.Mappings")

	if err := query.Count(&totalCount).Error; err != nil {
		return 0, 0, err
	}

	var totalPages int
	if pageSize > 0 {
		totalPages = int(totalCount) / pageSize
		if int(totalCount)%pageSize != 0 {
			totalPages++
		}
	}

	if !all && pageSize > 0 {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Find(r).Error
	return totalCount, totalPages, err
}

// GetByNamespace retrieves all ModelRouters for a specific namespace
func (r *ModelRouters) GetByNamespace(db *gorm.DB, namespace string) error {
	return db.Preload("Pools.Vendors.LLM").Preload("Pools.Vendors.Mappings").
		Where("namespace = ?", namespace).Find(r).Error
}

// GetActiveRouters retrieves all active ModelRouters
func (r *ModelRouters) GetActiveRouters(db *gorm.DB) error {
	return db.Preload("Pools.Vendors.LLM").Preload("Pools.Vendors.Mappings").
		Where("active = ?", true).Find(r).Error
}

// GetActiveRoutersByNamespace retrieves all active ModelRouters for a namespace
func (r *ModelRouters) GetActiveRoutersByNamespace(db *gorm.DB, namespace string) error {
	return db.Preload("Pools.Vendors.LLM").Preload("Pools.Vendors.Mappings").
		Where("active = ? AND namespace = ?", true, namespace).Find(r).Error
}

// NewModelPool creates a new ModelPool instance
func NewModelPool() *ModelPool {
	return &ModelPool{}
}

// Get retrieves a ModelPool by ID with relationships
func (p *ModelPool) Get(db *gorm.DB, id uint) error {
	return db.Preload("Vendors.LLM").Preload("Vendors.Mappings").First(p, id).Error
}

// GetByRouterID retrieves all pools for a router, ordered by priority
func (p *ModelPools) GetByRouterID(db *gorm.DB, routerID uint) error {
	return db.Preload("Vendors.LLM").Preload("Vendors.Mappings").
		Where("router_id = ?", routerID).
		Order("priority DESC").Find(p).Error
}

// NewPoolVendor creates a new PoolVendor instance
func NewPoolVendor() *PoolVendor {
	return &PoolVendor{}
}

// Get retrieves a PoolVendor by ID with LLM and Mappings relationships
func (v *PoolVendor) Get(db *gorm.DB, id uint) error {
	return db.Preload("LLM").Preload("Mappings").First(v, id).Error
}

// GetActiveVendorsByPoolID retrieves all active vendors for a pool
func (v *PoolVendors) GetActiveVendorsByPoolID(db *gorm.DB, poolID uint) error {
	return db.Preload("LLM").Preload("Mappings").
		Where("pool_id = ? AND active = ?", poolID, true).Find(v).Error
}

// NewModelMapping creates a new ModelMapping instance
func NewModelMapping() *ModelMapping {
	return &ModelMapping{}
}

// Get retrieves a ModelMapping by ID
func (m *ModelMapping) Get(db *gorm.DB, id uint) error {
	return db.First(m, id).Error
}

// GetByVendorID retrieves all mappings for a vendor
func (m *ModelMappings) GetByVendorID(db *gorm.DB, vendorID uint) error {
	return db.Where("vendor_id = ?", vendorID).Find(m).Error
}

// GetMappingForModel finds a mapping for a specific source model for a vendor
func (m *ModelMapping) GetMappingForModel(db *gorm.DB, vendorID uint, sourceModel string) error {
	return db.Where("vendor_id = ? AND source_model = ?", vendorID, sourceModel).First(m).Error
}
