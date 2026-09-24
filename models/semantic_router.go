package models

import (
	"encoding/json"
	"fmt"

	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"gorm.io/gorm"
)

// SemanticRouter is a router that picks one of its named routes from what the
// prompt says (Enterprise; see pkg/semanticrouting). Like a Model Router it is
// addressed on the unified ingress ("{slug}/auto"), published in LLM
// catalogues and granted to Apps; a grant reaches the LLMs its routes send to,
// only through the router.
type SemanticRouter struct {
	gorm.Model
	ID          uint   `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"not null"`
	Slug        string `json:"slug" gorm:"uniqueIndex:idx_semantic_router_slug_namespace;not null"`
	Namespace   string `json:"namespace" gorm:"default:'';uniqueIndex:idx_semantic_router_slug_namespace;index:idx_semantic_router_namespace"`
	Description string `json:"description"`
	Active      bool   `json:"active" gorm:"default:false"`
	// APICompat is the API the router is called with. Only the OpenAI-shaped
	// unified ingress routes today.
	APICompat string `json:"api_compat" gorm:"default:'openai'"`

	// Portal presentation.
	ShortDescription string `json:"short_description"`
	LongDescription  string `json:"long_description"`
	LogoURL          string `json:"logo_url"`

	Settings sr.Settings `json:"settings" gorm:"serializer:json"`
	Routes   []sr.Route  `json:"routes" gorm:"serializer:json"`

	// EmbedderID is the embedder of the embedding stage (examples and
	// requests). Settings.Embedding then only carries its timeout; without
	// an embedder, a Settings.Embedding naming an LLM is used as it is
	// (routers saved before Embedders, draft tests).
	EmbedderID *uint     `json:"embedder_id" gorm:"index"`
	Embedder   *Embedder `json:"-" gorm:"foreignKey:EmbedderID"`

	Catalogues []Catalogue `json:"-" gorm:"many2many:catalogue_semantic_routers;"`
}

// SemanticRouterTarget records what a router may send a request's text to:
// the LLMs and Model Routers its routes target, and the LLMs that embed and
// judge. It is rebuilt whenever the router is saved, so the privacy score
// (the least private of them) and "what uses this LLM" can be answered in SQL.
type SemanticRouterTarget struct {
	ID            uint   `gorm:"primaryKey"`
	RouterID      uint   `gorm:"not null;index:idx_semantic_router_target_router"`
	LLMID         *uint  `gorm:"index:idx_semantic_router_target_llm"`
	ModelRouterID *uint  `gorm:"index:idx_semantic_router_target_model_router"`
	Role          string `gorm:"size:16;not null"` // "route" or "classifier"
}

// Roles of a SemanticRouterTarget.
const (
	SemanticTargetRoute      = "route"
	SemanticTargetClassifier = "classifier"
)

// Config is the router as the engine compiles it.
func (r *SemanticRouter) Config() sr.Config {
	return sr.Config{RouterID: r.ID, Slug: r.Slug, Settings: r.Settings, Routes: r.Routes}
}

// ConfigJSON is Config as stored, without resolving the embedder.
func (r *SemanticRouter) ConfigJSON() (string, error) {
	b, err := json.Marshal(r.Config())
	return string(b), err
}

// CompiledConfig is the router as the engine runs it: the embedder is
// flattened into Settings.Embedding. A linked embedder becomes the LLM
// reference older edges understand ({llm_id, model}); a standalone one is
// carried inline. With encrypt nil (the hub) the inline key is resolved into
// APIKey; with encrypt set (the edge snapshot) it travels encrypted in
// APIKeyEncrypted. A router without an embedder keeps its Settings.Embedding.
func (r *SemanticRouter) CompiledConfig(db *gorm.DB, encrypt func(string) (string, error)) (sr.Config, error) {
	cfg := r.Config()
	if r.EmbedderID == nil {
		return cfg, nil
	}
	e := r.Embedder
	if e == nil || e.ID != *r.EmbedderID || (e.IsLinked() && e.LLM == nil) {
		var loaded Embedder
		if err := loaded.Get(db, *r.EmbedderID); err != nil {
			return cfg, fmt.Errorf("embedder %d: %w", *r.EmbedderID, err)
		}
		e = &loaded
	}
	timeout := 0
	if r.Settings.Embedding != nil {
		timeout = r.Settings.Embedding.TimeoutMs
	}
	if e.IsLinked() {
		if e.LLM == nil {
			return cfg, ErrEmbedderLLMMissing
		}
		cfg.Settings.Embedding = &sr.ModelRef{LLMID: *e.LLMID, Model: e.ModelName, TimeoutMs: timeout}
		return cfg, nil
	}
	spec, err := e.Spec(true)
	if err != nil {
		return cfg, err
	}
	ref := &sr.ModelRef{
		Model:      spec.Model,
		TimeoutMs:  timeout,
		EmbedderID: spec.EmbedderID,
		Vendor:     string(spec.Vendor),
		Endpoint:   spec.Endpoint,
	}
	if encrypt == nil {
		ref.APIKey = spec.APIKey
	} else if spec.APIKey != "" {
		enc, err := encrypt(spec.APIKey)
		if err != nil {
			return cfg, fmt.Errorf("encrypt embedder key: %w", err)
		}
		ref.APIKeyEncrypted = enc
	}
	cfg.Settings.Embedding = ref
	return cfg, nil
}

// EdgeConfigJSON is CompiledConfig as the snapshot carries it to the edge.
// The encoding is deterministic (struct field order) and the key encryption
// is too, so the snapshot checksum is stable.
func (r *SemanticRouter) EdgeConfigJSON(db *gorm.DB, encrypt func(string) (string, error)) (string, error) {
	cfg, err := r.CompiledConfig(db, encrypt)
	if err != nil {
		return "", err
	}
	b, err := json.Marshal(cfg)
	return string(b), err
}

// targets derives the target rows from the router's configuration.
func (r *SemanticRouter) targets() []SemanticRouterTarget {
	var out []SemanticRouterTarget
	seenLLM := map[string]bool{}
	seenMR := map[uint]bool{}
	addLLM := func(id uint, role string) {
		key := fmt.Sprintf("%d|%s", id, role)
		if id == 0 || seenLLM[key] {
			return
		}
		seenLLM[key] = true
		v := id
		out = append(out, SemanticRouterTarget{RouterID: r.ID, LLMID: &v, Role: role})
	}
	for _, rt := range r.Routes {
		switch rt.Target.Type {
		case sr.TargetLLM:
			addLLM(rt.Target.LLMID, SemanticTargetRoute)
		case sr.TargetModelRouter:
			if id := rt.Target.ModelRouterID; id != 0 && !seenMR[id] {
				seenMR[id] = true
				v := id
				out = append(out, SemanticRouterTarget{RouterID: r.ID, ModelRouterID: &v, Role: SemanticTargetRoute})
			}
		}
	}
	if r.Embedder != nil {
		if r.Embedder.IsLinked() {
			addLLM(*r.Embedder.LLMID, SemanticTargetClassifier)
		}
	} else if e := r.Settings.Embedding; e != nil && r.EmbedderID == nil {
		addLLM(e.LLMID, SemanticTargetClassifier)
	}
	if r.Settings.Judge.Enabled {
		addLLM(r.Settings.Judge.ModelRef.LLMID, SemanticTargetClassifier)
	}
	return out
}

func (r *SemanticRouter) writeTargets(tx *gorm.DB) error {
	if r.EmbedderID != nil && (r.Embedder == nil || r.Embedder.ID != *r.EmbedderID) {
		var e Embedder
		if err := tx.First(&e, *r.EmbedderID).Error; err != nil {
			return fmt.Errorf("embedder %d: %w", *r.EmbedderID, err)
		}
		r.Embedder = &e
	} else if r.EmbedderID == nil {
		r.Embedder = nil
	}
	if err := tx.Where("router_id = ?", r.ID).Delete(&SemanticRouterTarget{}).Error; err != nil {
		return err
	}
	if t := r.targets(); len(t) > 0 {
		return tx.Create(&t).Error
	}
	return nil
}

// Get loads a router by id, with its catalogues.
func (r *SemanticRouter) Get(db *gorm.DB, id uint) error {
	return db.Preload("Catalogues").Preload("Embedder.LLM").First(r, id).Error
}

// Create inserts the router and its target rows.
func (r *SemanticRouter) Create(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Catalogues", "Embedder").Create(r).Error; err != nil {
			return err
		}
		return r.writeTargets(tx)
	})
}

// Update saves the router's fields (not its catalogues) and rebuilds its
// target rows.
func (r *SemanticRouter) Update(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Catalogues", "Embedder", "CreatedAt").Save(r).Error; err != nil {
			return err
		}
		return r.writeTargets(tx)
	})
}

// Delete removes the router for good, with its targets, App grants and
// catalogue memberships. It is a hard delete: a soft-deleted row would keep
// its slug in the unique index and block a new router of the same name.
func (r *SemanticRouter) Delete(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"app_semantic_routers", "catalogue_semantic_routers"} {
			if err := tx.Exec("DELETE FROM "+table+" WHERE semantic_router_id = ?", r.ID).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("router_id = ?", r.ID).Delete(&SemanticRouterTarget{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&SemanticRouter{}, r.ID).Error
	})
}

// SemanticRouters is a list of routers.
type SemanticRouters []SemanticRouter

// GetAll lists routers, paged unless all is set.
func (rs *SemanticRouters) GetAll(db *gorm.DB, pageSize, pageNumber int, all bool, scopes ...func(*gorm.DB) *gorm.DB) (int64, int, error) {
	var total int64
	q := db.Model(&SemanticRouter{}).Preload("Catalogues").Preload("Embedder.LLM")
	for _, s := range scopes {
		q = s(q)
	}
	if err := q.Count(&total).Error; err != nil {
		return 0, 0, err
	}
	totalPages := 1
	if !all && pageSize > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
		if pageNumber < 1 {
			pageNumber = 1
		}
		q = q.Offset((pageNumber - 1) * pageSize).Limit(pageSize)
	}
	return total, totalPages, q.Order("id ASC").Find(rs).Error
}

// SemanticRouterPrivacySQL is a router's privacy score in SQL, as a
// correlated subquery on semantic_routers.id: the lowest score among the
// active LLMs the router may send a request's text to (its LLM targets, the
// active vendors of the Model Routers it hands off to, and its embedding and
// judge LLMs). A Model Router target counts with all of its vendors, which is
// never less strict than the pool a given alias reaches.
//
// A standalone embedder sees the text too, with its own privacy score, so it
// counts as well (a linked one is already among the LLM targets).
const SemanticRouterPrivacySQL = "(SELECT MIN(rs.score) FROM (" +
	"SELECT sl.privacy_score AS score FROM llms sl " +
	"WHERE sl.active = TRUE AND sl.deleted_at IS NULL AND sl.id IN (" +
	"SELECT st.llm_id FROM semantic_router_targets st WHERE st.router_id = semantic_routers.id AND st.llm_id IS NOT NULL " +
	"UNION SELECT spv.llm_id FROM semantic_router_targets st2 " +
	"JOIN model_pools smp ON smp.router_id = st2.model_router_id AND smp.deleted_at IS NULL " +
	"JOIN pool_vendors spv ON spv.pool_id = smp.id AND spv.deleted_at IS NULL AND spv.active = TRUE " +
	"WHERE st2.router_id = semantic_routers.id) " +
	"UNION ALL SELECT se.privacy_score AS score FROM embedders se " +
	"WHERE se.id = semantic_routers.embedder_id AND se.llm_id IS NULL AND se.deleted_at IS NULL" +
	") rs)"

// SemanticRouterPrivacyScores maps router ids to their privacy score (see
// SemanticRouterPrivacySQL). Routers that reach no active LLM are absent.
func SemanticRouterPrivacyScores(db *gorm.DB, ids []uint) (map[uint]int, error) {
	out := map[uint]int{}
	if len(ids) == 0 {
		return out, nil
	}
	var rows []struct {
		ID    uint
		Score *int
	}
	if err := db.Model(&SemanticRouter{}).
		Select("semantic_routers.id AS id, "+SemanticRouterPrivacySQL+" AS score").
		Where("semantic_routers.id IN ?", ids).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if r.Score != nil {
			out[r.ID] = *r.Score
		}
	}
	return out, nil
}

// SemanticRouterReachableLLMs lists the active LLMs a router's routes can
// send a request to: its LLM targets and the active vendors of the Model
// Routers it hands off to. (Its embedding and judge LLMs see the prompt but
// never answer it, so they are not listed here.)
func SemanticRouterReachableLLMs(db *gorm.DB, routerID uint) ([]LLM, error) {
	var llms []LLM
	err := db.Model(&LLM{}).
		Where("llms.active = ? AND llms.id IN (?)", true,
			db.Raw("SELECT st.llm_id FROM semantic_router_targets st WHERE st.router_id = ? AND st.role = ? AND st.llm_id IS NOT NULL "+
				"UNION SELECT spv.llm_id FROM semantic_router_targets st2 "+
				"JOIN model_pools smp ON smp.router_id = st2.model_router_id AND smp.deleted_at IS NULL "+
				"JOIN pool_vendors spv ON spv.pool_id = smp.id AND spv.deleted_at IS NULL AND spv.active = ? "+
				"WHERE st2.router_id = ? AND st2.role = ?",
				routerID, SemanticTargetRoute, true, routerID, SemanticTargetRoute)).
		Order("llms.name").
		Find(&llms).Error
	return llms, err
}

// SemanticRouterCatalogueMemberships maps router ids to the LLM catalogues
// (among catalogueIDs) they belong to.
func SemanticRouterCatalogueMemberships(db *gorm.DB, catalogueIDs []uint) (map[uint][]uint, error) {
	return catalogueMemberships(db, "catalogue_semantic_routers", "catalogue_id", "semantic_router_id", catalogueIDs)
}

// AccessibleSemanticRouterQuery is the portal visibility rule for Semantic
// Routers: the user's teams -> their LLM catalogues -> active routers in them.
func AccessibleSemanticRouterQuery(db *gorm.DB, userID uint) *gorm.DB {
	return db.Model(&SemanticRouter{}).
		Joins("JOIN catalogue_semantic_routers ON catalogue_semantic_routers.semantic_router_id = semantic_routers.id").
		Joins("JOIN catalogues ON catalogues.id = catalogue_semantic_routers.catalogue_id AND catalogues.deleted_at IS NULL").
		Joins("JOIN group_catalogues ON group_catalogues.catalogue_id = catalogues.id").
		Joins("JOIN user_groups ON user_groups.group_id = group_catalogues.group_id").
		Where("user_groups.user_id = ? AND semantic_routers.active = ?", userID, true)
}
