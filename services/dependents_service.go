package services

import (
	"strconv"

	"github.com/TykTechnologies/midsommar/v2/models"
	"gorm.io/gorm"
)

// DependentRef identifies one object that references another.
type DependentRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// Dependents lists every object that references a given object, grouped by
// type. Every slice is non-nil so the JSON always carries every array, and
// Total is the sum of their lengths.
//
// This is what the delete confirmation shows ("used by 2 apps and 1
// catalogue") and what the object page uses to explain where something is
// exposed. The queries are plain joins on the association tables so they run
// the same on SQLite and Postgres.
type Dependents struct {
	Apps         []DependentRef `json:"apps"`
	Catalogues   []DependentRef `json:"catalogues"`
	LLMs         []DependentRef `json:"llms"`
	Tools        []DependentRef `json:"tools"`
	Datasources  []DependentRef `json:"datasources"`
	Agents       []DependentRef `json:"agents"`
	ModelRouters []DependentRef `json:"model_routers"`
	// SemanticRouters that route to, hand off to, or classify with the object.
	SemanticRouters []DependentRef `json:"semantic_routers"`
	Chats           []DependentRef `json:"chats"`
	// Embedders linked to the object (an LLM).
	Embedders []DependentRef `json:"embedders"`
	Total     int            `json:"total"`
}

func newDependents() *Dependents {
	return &Dependents{
		Apps:            []DependentRef{},
		Catalogues:      []DependentRef{},
		LLMs:            []DependentRef{},
		Tools:           []DependentRef{},
		Datasources:     []DependentRef{},
		Agents:          []DependentRef{},
		ModelRouters:    []DependentRef{},
		SemanticRouters: []DependentRef{},
		Chats:           []DependentRef{},
		Embedders:       []DependentRef{},
	}
}

func (d *Dependents) finalize() *Dependents {
	d.Total = len(d.Apps) + len(d.Catalogues) + len(d.LLMs) + len(d.Tools) +
		len(d.Datasources) + len(d.Agents) + len(d.ModelRouters) + len(d.SemanticRouters) + len(d.Chats) + len(d.Embedders)
	return d
}

// SecretReference is one object that reads a secret through a $SECRET/<name>
// reference. Type is "llm", "tool" or "datasource".
type SecretReference struct {
	Type string `json:"type"`
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// dependentRefs runs one "who references X" join and returns id/name pairs.
//
// model carries the schema (so GORM's soft-delete scope applies to the
// referencing table), table is that model's table name, joins the JOIN
// clause(s) onto the association table, and where/args the predicate on it.
// Results are grouped so a join that can match twice (a router with the same
// LLM in two pools) yields one row, and ordered by id for stable output.
func dependentRefs(db *gorm.DB, model interface{}, table, joins, where string, args ...interface{}) ([]DependentRef, error) {
	refs := []DependentRef{}
	q := db.Model(model).Select(table + ".id AS id, " + table + ".name AS name")
	if joins != "" {
		q = q.Joins(joins)
	}
	err := q.Where(where, args...).
		Group(table + ".id, " + table + ".name").
		Order(table + ".id").
		Scan(&refs).Error
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// GetLLMDependents lists the apps, catalogues, agents, failover primaries and
// model routers that reference an LLM.
func (s *Service) GetLLMDependents(llmID uint) (*Dependents, error) {
	d := newDependents()
	var err error

	if d.Apps, err = dependentRefs(s.DB, &models.App{}, "apps",
		"JOIN app_llms ON app_llms.app_id = apps.id",
		"app_llms.llm_id = ?", llmID); err != nil {
		return nil, err
	}
	if d.Catalogues, err = dependentRefs(s.DB, &models.Catalogue{}, "catalogues",
		"JOIN catalogue_llms ON catalogue_llms.catalogue_id = catalogues.id",
		"catalogue_llms.llm_id = ?", llmID); err != nil {
		return nil, err
	}
	// Agents reach an LLM through their app, so an agent whose app includes
	// the LLM stops working when the LLM goes.
	if d.Agents, err = dependentRefs(s.DB, &models.AgentConfig{}, "agent_configs",
		"JOIN app_llms ON app_llms.app_id = agent_configs.app_id",
		"app_llms.llm_id = ?", llmID); err != nil {
		return nil, err
	}
	if d.ModelRouters, err = dependentRefs(s.DB, &models.ModelRouter{}, "model_routers",
		"JOIN model_pools ON model_pools.router_id = model_routers.id AND model_pools.deleted_at IS NULL "+
			"JOIN pool_vendors ON pool_vendors.pool_id = model_pools.id AND pool_vendors.deleted_at IS NULL",
		"pool_vendors.llm_id = ?", llmID); err != nil {
		return nil, err
	}
	if d.SemanticRouters, err = dependentRefs(s.DB, &models.SemanticRouter{}, "semantic_routers",
		"JOIN semantic_router_targets ON semantic_router_targets.router_id = semantic_routers.id",
		"semantic_router_targets.llm_id = ?", llmID); err != nil {
		return nil, err
	}
	if d.Chats, err = dependentRefs(s.DB, &models.Chat{}, "chats", "",
		"chats.llm_id = ?", llmID); err != nil {
		return nil, err
	}
	if d.Embedders, err = dependentRefs(s.DB, &models.Embedder{}, "embedders", "",
		"embedders.llm_id = ?", llmID); err != nil {
		return nil, err
	}

	// Failover targets live in a JSON column. Querying JSON portably across
	// SQLite and Postgres is not worth it for a table this small, so scan the
	// rows that have a waterfall at all and match in Go.
	// The LIKE pre-filter keeps the scan to rows whose serialised waterfall
	// mentions this id at all (json.Marshal writes `"llm_id":3,` or
	// `"llm_id":3}`); the Go match below is still what decides, so a false
	// positive from the text match (e.g. inside a model name) is harmless.
	// TODO: switch to native JSON operators (Postgres jsonb @>) if failover
	// tables grow large enough for this to show up; SQLite keeps it portable.
	idText := strconv.FormatUint(uint64(llmID), 10)
	var primaries []models.LLM
	if err := s.DB.Select("id", "name", "failover").
		Where("failover IS NOT NULL AND id <> ?", llmID).
		Where("CAST(failover AS TEXT) LIKE ? OR CAST(failover AS TEXT) LIKE ?",
			"%\"llm_id\":"+idText+",%", "%\"llm_id\":"+idText+"}%").
		Order("id").
		Find(&primaries).Error; err != nil {
		return nil, err
	}
	for _, primary := range primaries {
		for _, target := range primary.Failover.Targets {
			if target.LLMID == llmID {
				d.LLMs = append(d.LLMs, DependentRef{ID: primary.ID, Name: primary.Name})
				break
			}
		}
	}

	return d.finalize(), nil
}

// GetToolDependents lists the apps, tool catalogues, dependent tools, agents
// and chats that reference a tool.
func (s *Service) GetToolDependents(toolID uint) (*Dependents, error) {
	d := newDependents()
	var err error

	if d.Apps, err = dependentRefs(s.DB, &models.App{}, "apps",
		"JOIN app_tools ON app_tools.app_id = apps.id",
		"app_tools.tool_id = ?", toolID); err != nil {
		return nil, err
	}
	if d.Catalogues, err = dependentRefs(s.DB, &models.ToolCatalogue{}, "tool_catalogues",
		"JOIN tool_catalogue_tools ON tool_catalogue_tools.tool_catalogue_id = tool_catalogues.id",
		"tool_catalogue_tools.tool_id = ?", toolID); err != nil {
		return nil, err
	}
	// tool_dependencies rows read "tool_id depends on dependency_id".
	if d.Tools, err = dependentRefs(s.DB, &models.Tool{}, "tools",
		"JOIN tool_dependencies ON tool_dependencies.tool_id = tools.id",
		"tool_dependencies.dependency_id = ?", toolID); err != nil {
		return nil, err
	}
	if d.Agents, err = dependentRefs(s.DB, &models.AgentConfig{}, "agent_configs",
		"JOIN app_tools ON app_tools.app_id = agent_configs.app_id",
		"app_tools.tool_id = ?", toolID); err != nil {
		return nil, err
	}
	if d.Chats, err = dependentRefs(s.DB, &models.Chat{}, "chats",
		"JOIN chat_tools ON chat_tools.chat_id = chats.id",
		"chat_tools.tool_id = ?", toolID); err != nil {
		return nil, err
	}

	return d.finalize(), nil
}

// GetDatasourceDependents lists the apps, data catalogues, agents and chats
// that reference a datasource.
func (s *Service) GetDatasourceDependents(datasourceID uint) (*Dependents, error) {
	d := newDependents()
	var err error

	if d.Apps, err = dependentRefs(s.DB, &models.App{}, "apps",
		"JOIN app_datasources ON app_datasources.app_id = apps.id",
		"app_datasources.datasource_id = ?", datasourceID); err != nil {
		return nil, err
	}
	if d.Catalogues, err = dependentRefs(s.DB, &models.DataCatalogue{}, "data_catalogues",
		"JOIN data_catalogue_data_sources ON data_catalogue_data_sources.data_catalogue_id = data_catalogues.id",
		"data_catalogue_data_sources.datasource_id = ?", datasourceID); err != nil {
		return nil, err
	}
	if d.Agents, err = dependentRefs(s.DB, &models.AgentConfig{}, "agent_configs",
		"JOIN app_datasources ON app_datasources.app_id = agent_configs.app_id",
		"app_datasources.datasource_id = ?", datasourceID); err != nil {
		return nil, err
	}
	if d.Chats, err = dependentRefs(s.DB, &models.Chat{}, "chats", "",
		"chats.default_data_source_id = ?", datasourceID); err != nil {
		return nil, err
	}

	return d.finalize(), nil
}

// GetEmbedderDependents lists the datasources and Semantic Routers that
// embed with an embedder.
func (s *Service) GetEmbedderDependents(embedderID uint) (*Dependents, error) {
	d := newDependents()
	var err error

	if d.Datasources, err = dependentRefs(s.DB, &models.Datasource{}, "datasources", "",
		"datasources.embedder_id = ?", embedderID); err != nil {
		return nil, err
	}
	if d.SemanticRouters, err = dependentRefs(s.DB, &models.SemanticRouter{}, "semantic_routers", "",
		"semantic_routers.embedder_id = ?", embedderID); err != nil {
		return nil, err
	}
	return d.finalize(), nil
}

// GetFilterDependents lists the LLMs, tools and chats a filter is attached to.
func (s *Service) GetFilterDependents(filterID uint) (*Dependents, error) {
	d := newDependents()
	var err error

	if d.LLMs, err = dependentRefs(s.DB, &models.LLM{}, "llms",
		"JOIN llm_filters ON llm_filters.llm_id = llms.id",
		"llm_filters.filter_id = ?", filterID); err != nil {
		return nil, err
	}
	if d.Tools, err = dependentRefs(s.DB, &models.Tool{}, "tools",
		"JOIN tool_filters ON tool_filters.tool_id = tools.id",
		"tool_filters.filter_id = ?", filterID); err != nil {
		return nil, err
	}
	if d.Chats, err = dependentRefs(s.DB, &models.Chat{}, "chats",
		"JOIN chat_filters ON chat_filters.chat_id = chats.id",
		"chat_filters.filter_id = ?", filterID); err != nil {
		return nil, err
	}

	return d.finalize(), nil
}

// GetModelRouterDependents lists what references a model router: the apps
// granted it and the LLM catalogues it is published in. Deleting the router
// withdraws both (ModelRouter.Delete).
func (s *Service) GetModelRouterDependents(routerID uint) (*Dependents, error) {
	d := newDependents()
	var err error

	if d.Apps, err = dependentRefs(s.DB, &models.App{}, "apps",
		"JOIN app_model_routers ON app_model_routers.app_id = apps.id",
		"app_model_routers.model_router_id = ?", routerID); err != nil {
		return nil, err
	}
	if d.Catalogues, err = dependentRefs(s.DB, &models.Catalogue{}, "catalogues",
		"JOIN catalogue_model_routers ON catalogue_model_routers.catalogue_id = catalogues.id",
		"catalogue_model_routers.model_router_id = ?", routerID); err != nil {
		return nil, err
	}
	if d.SemanticRouters, err = dependentRefs(s.DB, &models.SemanticRouter{}, "semantic_routers",
		"JOIN semantic_router_targets ON semantic_router_targets.router_id = semantic_routers.id",
		"semantic_router_targets.model_router_id = ?", routerID); err != nil {
		return nil, err
	}
	return d.finalize(), nil
}

// GetSemanticRouterDependents lists what references a semantic router: the
// apps granted it and the LLM catalogues it is published in. Deleting the
// router withdraws both (SemanticRouter.Delete).
func (s *Service) GetSemanticRouterDependents(routerID uint) (*Dependents, error) {
	d := newDependents()
	var err error

	if d.Apps, err = dependentRefs(s.DB, &models.App{}, "apps",
		"JOIN app_semantic_routers ON app_semantic_routers.app_id = apps.id",
		"app_semantic_routers.semantic_router_id = ?", routerID); err != nil {
		return nil, err
	}
	if d.Catalogues, err = dependentRefs(s.DB, &models.Catalogue{}, "catalogues",
		"JOIN catalogue_semantic_routers ON catalogue_semantic_routers.catalogue_id = catalogues.id",
		"catalogue_semantic_routers.semantic_router_id = ?", routerID); err != nil {
		return nil, err
	}
	return d.finalize(), nil
}

// GetAppDependents lists what references an app: the agents configured to
// run as it (agent_configs.app_id). Chats do not reference apps, and an
// app's plugin resource bindings (app_plugin_resources) are its own
// configuration, cleared by DeleteApp, rather than dependents.
func (s *Service) GetAppDependents(appID uint) (*Dependents, error) {
	d := newDependents()
	var err error

	if d.Agents, err = dependentRefs(s.DB, &models.AgentConfig{}, "agent_configs",
		"", "agent_configs.app_id = ?", appID); err != nil {
		return nil, err
	}
	return d.finalize(), nil
}

// GetSecretDependents lists the LLMs, tools and datasources that read a
// secret through a $SECRET/<name> reference.
func (s *Service) GetSecretDependents(varName string) (*Dependents, error) {
	refs, err := s.secretReferences([]string{varName})
	if err != nil {
		return nil, err
	}

	d := newDependents()
	for _, ref := range refs[varName] {
		item := DependentRef{ID: ref.ID, Name: ref.Name}
		switch ref.Type {
		case "llm":
			d.LLMs = append(d.LLMs, item)
		case "tool":
			d.Tools = append(d.Tools, item)
		case "datasource":
			d.Datasources = append(d.Datasources, item)
		case models.SecretRefObjectEmbedder:
			d.Embedders = append(d.Embedders, item)
		}
	}
	return d.finalize(), nil
}

// SecretReferences returns, keyed by secret name, the objects that read
// each secret through a $SECRET/<name> reference. It is served from the
// secret_references index that the LLM, tool and datasource model hooks
// maintain (see models.SecretReference), so the secrets list is one indexed
// query however many objects exist.
func (s *Service) SecretReferences() (map[string][]SecretReference, error) {
	return s.secretReferences(nil)
}

// SecretReferencesByNames is SecretReferences restricted to the given secret
// names: the secrets list calls it for the page it is about to render, so
// the query is bounded by the page size rather than by the whole index.
func (s *Service) SecretReferencesByNames(names []string) (map[string][]SecretReference, error) {
	if len(names) == 0 {
		return map[string][]SecretReference{}, nil
	}
	return s.secretReferences(names)
}

// secretReferences is SecretReferences narrowed to the given secret names;
// an empty list means every secret.
func (s *Service) secretReferences(names []string) (map[string][]SecretReference, error) {
	q := s.DB.Model(&models.SecretReference{}).
		Order("secret_name, object_type, object_id")
	if len(names) > 0 {
		q = q.Where("secret_name IN ?", names)
	}
	var rows []models.SecretReference
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string][]SecretReference{}
	for _, r := range rows {
		out[r.SecretName] = append(out[r.SecretName], SecretReference{Type: r.ObjectType, ID: r.ObjectID, Name: r.ObjectName})
	}
	return out, nil
}
