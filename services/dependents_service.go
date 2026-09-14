package services

import (
	"sort"
	"strconv"
	"strings"

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
	Chats        []DependentRef `json:"chats"`
	Total        int            `json:"total"`
}

func newDependents() *Dependents {
	return &Dependents{
		Apps:         []DependentRef{},
		Catalogues:   []DependentRef{},
		LLMs:         []DependentRef{},
		Tools:        []DependentRef{},
		Datasources:  []DependentRef{},
		Agents:       []DependentRef{},
		ModelRouters: []DependentRef{},
		Chats:        []DependentRef{},
	}
}

func (d *Dependents) finalize() *Dependents {
	d.Total = len(d.Apps) + len(d.Catalogues) + len(d.LLMs) + len(d.Tools) +
		len(d.Datasources) + len(d.Agents) + len(d.ModelRouters) + len(d.Chats)
	return d
}

// SecretReference is one object that reads a secret through a $SECRET/<name>
// reference. Type is "llm", "tool" or "datasource".
type SecretReference struct {
	Type string `json:"type"`
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

const secretRefPrefix = "$SECRET/"

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
	if d.Chats, err = dependentRefs(s.DB, &models.Chat{}, "chats", "",
		"chats.llm_id = ?", llmID); err != nil {
		return nil, err
	}

	// Failover targets live in a JSON column. Querying JSON portably across
	// SQLite and Postgres is not worth it for a table this small, so scan the
	// rows that have a waterfall at all and match in Go.
	var primaries []models.LLM
	if err := s.DB.Select("id", "name", "failover").
		Where("failover IS NOT NULL AND id <> ?", llmID).
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

// GetModelRouterDependents lists what references a model router.
//
// Nothing in the data model points at a router today: routers are addressed
// by slug on the gateway (/router/{slug}/...) and apps carry no router
// association, so the answer is always empty. The endpoint exists so the
// delete dialog can ask one question for every object type.
func (s *Service) GetModelRouterDependents(routerID uint) (*Dependents, error) {
	return newDependents().finalize(), nil
}

// GetSecretDependents lists the LLMs, tools and datasources that read a
// secret through a $SECRET/<name> reference.
func (s *Service) GetSecretDependents(varName string) (*Dependents, error) {
	refs, err := s.SecretReferences()
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
		}
	}
	return d.finalize(), nil
}

// SecretReferences scans every credential field that may hold a
// $SECRET/<name> reference and returns, keyed by secret name, the objects
// that reference it. One scan serves both the secrets list (which needs the
// consumers of every secret) and the per-secret dependents endpoint.
//
// Fields checked: LLM api_key, api_endpoint and string metadata values
// (Bedrock keys live there); tool auth_key; datasource db_conn_api_key and
// embed_api_key. Each object appears at most once per secret.
func (s *Service) SecretReferences() (map[string][]SecretReference, error) {
	out := map[string][]SecretReference{}
	seen := map[string]map[string]bool{} // secret name -> "type:id"

	add := func(name string, ref SecretReference) {
		key := ref.Type + ":" + strconv.FormatUint(uint64(ref.ID), 10)
		if seen[name] == nil {
			seen[name] = map[string]bool{}
		}
		if seen[name][key] {
			return
		}
		seen[name][key] = true
		out[name] = append(out[name], ref)
	}

	var llms []models.LLM
	if err := s.DB.Select("id", "name", "api_key", "api_endpoint", "metadata").Order("id").Find(&llms).Error; err != nil {
		return nil, err
	}
	for _, llm := range llms {
		ref := SecretReference{Type: "llm", ID: llm.ID, Name: llm.Name}
		for _, v := range []string{llm.APIKey, llm.APIEndpoint} {
			if name, ok := secretNameFromReference(v); ok {
				add(name, ref)
			}
		}
		for _, v := range llm.Metadata {
			if str, isStr := v.(string); isStr {
				if name, ok := secretNameFromReference(str); ok {
					add(name, ref)
				}
			}
		}
	}

	var tools []models.Tool
	if err := s.DB.Select("id", "name", "auth_key").Order("id").Find(&tools).Error; err != nil {
		return nil, err
	}
	for _, tool := range tools {
		if name, ok := secretNameFromReference(tool.AuthKey); ok {
			add(name, SecretReference{Type: "tool", ID: tool.ID, Name: tool.Name})
		}
	}

	var datasources []models.Datasource
	if err := s.DB.Select("id", "name", "db_conn_api_key", "embed_api_key").Order("id").Find(&datasources).Error; err != nil {
		return nil, err
	}
	for _, ds := range datasources {
		ref := SecretReference{Type: "datasource", ID: ds.ID, Name: ds.Name}
		for _, v := range []string{ds.DBConnAPIKey, ds.EmbedAPIKey} {
			if name, ok := secretNameFromReference(v); ok {
				add(name, ref)
			}
		}
	}

	for name := range out {
		refs := out[name]
		sort.SliceStable(refs, func(i, j int) bool {
			if refs[i].Type != refs[j].Type {
				return refs[i].Type < refs[j].Type
			}
			return refs[i].ID < refs[j].ID
		})
	}
	return out, nil
}

// secretNameFromReference returns the secret name a $SECRET/<name> reference
// points at. Anything else (inline keys, $ENV/ references, empty) is not a
// secret reference.
func secretNameFromReference(value string) (string, bool) {
	if !strings.HasPrefix(value, secretRefPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(value, secretRefPrefix)
	if name == "" {
		return "", false
	}
	return name, true
}
