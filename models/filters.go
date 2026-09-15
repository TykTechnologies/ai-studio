package models

import (
	"github.com/TykTechnologies/midsommar/v2/guardrails"
	"gorm.io/gorm"
)

// Filter kinds. A script filter runs a Tengo script; a guardrail filter runs
// a typed provider (the built-in pattern library or an external classifier)
// described by Config. Both kinds share every attachment point and execution
// scope.
const (
	FilterKindScript    = "script"
	FilterKindGuardrail = "guardrail"
)

type Filter struct {
	gorm.Model
	ID             uint   `json:"id" gorm:"primaryKey"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	Script         []byte `json:"script"`
	ResponseFilter bool   `json:"response_filter" gorm:"default:false"` // true = response filter, false = request filter
	// Kind is FilterKindScript (default) or FilterKindGuardrail.
	Kind string `json:"kind" gorm:"default:'script';index:idx_filter_kind"`
	// Config is the guardrail configuration (guardrails.Config) when Kind is
	// FilterKindGuardrail; unused for scripts.
	Config JSONMap `json:"config" gorm:"type:json"`
	// Hub-and-Spoke Configuration
	Namespace string `json:"namespace" gorm:"default:'';index:idx_filter_namespace"`
}

// IsGuardrail reports whether the filter runs a provider rather than a script.
func (f *Filter) IsGuardrail() bool {
	return f.Kind == FilterKindGuardrail
}

// DefaultFilters are the guardrail filters seeded on first start. They use
// the built-in pattern library, so they work with no external service, and
// they are created unattached: a filter enforces nothing until an
// administrator attaches it to an LLM, chat or tool, which is the deliberate
// "present but inactive" state for a fresh install.
func DefaultFilters() []Filter {
	builtin := func(action string, detectors ...string) JSONMap {
		ds := make([]any, 0, len(detectors))
		for _, d := range detectors {
			ds = append(ds, map[string]any{"name": d})
		}
		return JSONMap{
			"provider":  "builtin",
			"detectors": ds,
			"on_detect": action,
		}
	}
	return []Filter{
		{
			Name:        "Credentials in prompts (block)",
			Description: "Blocks requests whose user messages carry API keys, access tokens, private keys or connection strings with passwords. Built-in pattern library, no external service.",
			Kind:        FilterKindGuardrail,
			Config:      builtin("block", "secrets"),
		},
		{
			Name:        "Personal data before the vendor (redact)",
			Description: "Redacts email addresses, phone numbers, national identifiers, payment cards and bank accounts from user messages before they leave for the LLM vendor. Built-in pattern library.",
			Kind:        FilterKindGuardrail,
			Config:      builtin("redact", "pii"),
		},
		{
			Name:        "Prompt injection heuristics (log)",
			Description: "Records a compliance event when a user message matches instruction-override, role-hijack or exfiltration heuristics. Logs only; pair with a classifier provider to block.",
			Kind:        FilterKindGuardrail,
			Config:      builtin("log", "injection"),
		},
		{
			Name:           "Credential leakage in responses (block)",
			Description:    "Stops an LLM response that contains an API key, token or private key, and records the event. Built-in pattern library.",
			Kind:           FilterKindGuardrail,
			ResponseFilter: true,
			Config:         builtin("block", "secrets", "leak"),
		},
	}
}

// GetOrCreateDefaultFilters seeds DefaultFilters by name, creating only the
// ones that do not exist yet so a renamed or edited filter is never
// overwritten and a deleted one is not resurrected on the next start (soft
// deletes are matched by Unscoped). Configs are stored normalised, as the
// admin API stores them, so edges receive explicit defaults.
func GetOrCreateDefaultFilters(db *gorm.DB) error {
	for _, f := range DefaultFilters() {
		var count int64
		if err := db.Unscoped().Model(&Filter{}).Where("name = ?", f.Name).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		filter := f
		cfg, err := guardrails.ParseConfig(filter.Config)
		if err != nil {
			return err
		}
		if cfg, err = guardrails.Normalize(cfg, filter.ResponseFilter); err != nil {
			return err
		}
		if filter.Config, err = cfg.ToMap(); err != nil {
			return err
		}
		if err := db.Create(&filter).Error; err != nil {
			return err
		}
	}
	return nil
}

func NewFilter() *Filter {
	return &Filter{}
}

// Create a new filter
func (f *Filter) Create(db *gorm.DB) error {
	return db.Create(f).Error
}

// Get a filter by ID
func (f *Filter) Get(db *gorm.DB, id uint) error {
	return db.First(f, id).Error
}

// Update an existing filter
func (f *Filter) Update(db *gorm.DB) error {
	return db.Save(f).Error
}

// Delete a filter
func (f *Filter) Delete(db *gorm.DB) error {
	return db.Delete(f).Error
}

// GetAll retrieves all filters
func (f *Filter) GetAll(db *gorm.DB, pageSize int, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) ([]Filter, int64, int, error) {
	var filters []Filter
	var totalCount int64
	query := db.Model(&Filter{})

	// Optional search/sort scopes (services.ListOptions); applied before the
	// count so X-Total-Count reflects the filtered set.
	for _, scope := range scopes {
		query = scope(query)
	}
	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Find(&filters).Error
	return filters, totalCount, totalPages, err
}

// GetAllWithFilters retrieves all filters with namespace filtering
func (f *Filter) GetAllWithFilters(db *gorm.DB, pageSize int, pageNumber int, all bool, namespace string) ([]Filter, int64, int, error) {
	var filters []Filter
	var totalCount int64
	query := db.Model(&Filter{})

	// Apply namespace filtering
	if namespace == "__ALL_NAMESPACES__" || namespace == "" {
		// No namespace filtering - return filters from all namespaces
		// No additional WHERE clause needed
	} else {
		// Specific namespace: only filters in specified namespace
		query = query.Where("namespace = ?", namespace)
	}

	if err := query.Count(&totalCount).Error; err != nil {
		return nil, 0, 0, err
	}

	totalPages := int(totalCount) / pageSize
	if int(totalCount)%pageSize != 0 {
		totalPages++
	}

	if !all {
		offset := (pageNumber - 1) * pageSize
		query = query.Offset(offset).Limit(pageSize)
	}

	err := query.Find(&filters).Error
	return filters, totalCount, totalPages, err
}

// GetByName gets a filter by its name
func (f *Filter) GetByName(db *gorm.DB, name string) error {
	return db.Where("name = ?", name).First(f).Error
}
