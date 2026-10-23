package models

import "github.com/TykTechnologies/midsommar/v2/guardrails"

// Tool, datasource and user credentials leave the process redacted, as LLM
// credentials do (RedactLLMCredentials): in system.tool, system.datasource
// and system.user events, which reach every subscribed plugin and the
// cluster_events table, and in every object hook. A literal value reads
// CredentialRedacted and a $SECRET/ or $ENV/ reference is kept. A
// before-hook's result gets the originals back wherever it still reads
// CredentialRedacted (RestoreObjectCredentials): the redaction means
// "unchanged", as on the API.

// redactLiteral replaces a literal credential with CredentialRedacted.
func redactLiteral(s *string) {
	if _, ok := LLMCredentialValue(*s); ok {
		*s = CredentialRedacted
	}
}

// restoreRedacted puts original back where modified still reads
// CredentialRedacted.
func restoreRedacted(modified *string, original string) {
	if *modified == CredentialRedacted {
		*modified = original
	}
}

// RedactFilterCredentials is a copy of f with the literal values of its
// guardrail connection's secret fields redacted (those the provider marks
// Secret, and any it does not declare); references and other fields
// (endpoints, regions) are kept. f is not changed (2ze).
func RedactFilterCredentials(f *Filter) *Filter {
	if f == nil {
		return nil
	}
	out := *f
	provider, _ := f.Config["provider"].(string)
	var conn map[string]any
	switch c := f.Config["connection"].(type) {
	case map[string]any:
		conn = c
	case map[string]string:
		conn = make(map[string]any, len(c))
		for k, v := range c {
			conn[k] = v
		}
	default:
		return &out
	}
	redacted := make(map[string]any, len(conn))
	for k, v := range conn {
		if s, ok := v.(string); ok && guardrails.IsSecretConnectionField(provider, k) {
			redactLiteral(&s)
			v = s
		}
		redacted[k] = v
	}
	out.Config = make(JSONMap, len(f.Config))
	for k, v := range f.Config {
		out.Config[k] = v
	}
	out.Config["connection"] = redacted
	return &out
}

// redactToolFilters and redactLLMFilters are RedactFilterCredentials for a
// tool's and an LLM's filters.
func redactToolFilters(filters []Filter) []Filter {
	if filters == nil {
		return nil
	}
	out := make([]Filter, len(filters))
	for i := range filters {
		out[i] = *RedactFilterCredentials(&filters[i])
	}
	return out
}

func redactLLMFilters(filters []*Filter) []*Filter {
	if filters == nil {
		return nil
	}
	out := make([]*Filter, len(filters))
	for i, f := range filters {
		out[i] = RedactFilterCredentials(f)
	}
	return out
}

// restoreFilterCredentials puts original's connection values back in
// modified wherever it still reads CredentialRedacted.
func restoreFilterCredentials(original, modified *Filter) {
	if original == nil || modified == nil {
		return
	}
	mc, ok := modified.Config["connection"].(map[string]any)
	if !ok {
		return
	}
	var orig map[string]any
	switch c := original.Config["connection"].(type) {
	case map[string]any:
		orig = c
	case map[string]string:
		orig = make(map[string]any, len(c))
		for k, v := range c {
			orig[k] = v
		}
	}
	for k, v := range mc {
		if s, ok := v.(string); ok && s == CredentialRedacted {
			if o, has := orig[k]; has {
				mc[k] = o
			}
		}
	}
}

// restoreFiltersByID restores each modified filter from the original with
// its ID.
func restoreFiltersByID(original, modified []*Filter) {
	byID := make(map[uint]*Filter, len(original))
	for _, f := range original {
		if f != nil {
			byID[f.ID] = f
		}
	}
	for _, f := range modified {
		if f != nil {
			restoreFilterCredentials(byID[f.ID], f)
		}
	}
}

func filterPtrs(filters []Filter) []*Filter {
	out := make([]*Filter, len(filters))
	for i := range filters {
		out[i] = &filters[i]
	}
	return out
}

// RedactToolCredentials is a copy of t with its auth key, its dependencies'
// auth keys and its filters' guardrail connection secrets redacted. t is not
// changed.
func RedactToolCredentials(t *Tool) *Tool {
	if t == nil {
		return nil
	}
	out := *t
	redactLiteral(&out.AuthKey)
	out.Filters = redactToolFilters(t.Filters)
	if t.Dependencies != nil {
		out.Dependencies = make([]*Tool, len(t.Dependencies))
		for i, dep := range t.Dependencies {
			out.Dependencies[i] = RedactToolCredentials(dep)
		}
	}
	return &out
}

// RedactDatasourceCredentials is a copy of d with its connection string,
// connection API key and embedder key redacted. The connection string is
// redacted whole, since it can carry a password. d is not changed.
func RedactDatasourceCredentials(d *Datasource) *Datasource {
	if d == nil {
		return nil
	}
	out := *d
	redactLiteral(&out.DBConnString)
	redactLiteral(&out.DBConnAPIKey)
	out.RedactEmbedKey()
	return &out
}

// RedactUserCredentials is a copy of u with its password hash, API key and
// session, reset and verification tokens redacted. u is not changed.
func RedactUserCredentials(u *User) *User {
	if u == nil {
		return nil
	}
	out := *u
	for _, f := range []*string{&out.Password, &out.APIKey, &out.SessionToken, &out.ResetToken, &out.VerificationToken} {
		redactLiteral(f)
	}
	return &out
}

func redactLLMs(llms []LLM) []LLM {
	if llms == nil {
		return nil
	}
	out := make([]LLM, len(llms))
	for i := range llms {
		out[i] = *RedactLLMCredentials(&llms[i])
	}
	return out
}

func redactTools(tools []Tool) []Tool {
	if tools == nil {
		return nil
	}
	out := make([]Tool, len(tools))
	for i := range tools {
		out[i] = *RedactToolCredentials(&tools[i])
	}
	return out
}

func redactDatasources(ds []Datasource) []Datasource {
	if ds == nil {
		return nil
	}
	out := make([]Datasource, len(ds))
	for i := range ds {
		out[i] = *RedactDatasourceCredentials(&ds[i])
	}
	return out
}

// RedactAppCredentials is a copy of a with its credential's secret (the
// App's gateway key) and the credentials of the LLMs, tools and datasources
// it preloads redacted. a is not changed (aia).
func RedactAppCredentials(a *App) *App {
	if a == nil {
		return nil
	}
	out := *a
	redactLiteral(&out.Credential.Secret)
	out.LLMs = redactLLMs(a.LLMs)
	out.Tools = redactTools(a.Tools)
	out.Datasources = redactDatasources(a.Datasources)
	return &out
}

// RedactGroupCredentials is a copy of g with its members' credentials, and
// those of the LLMs, datasources and tools its catalogues preload, redacted.
// g is not changed (aia).
func RedactGroupCredentials(g *Group) *Group {
	if g == nil {
		return nil
	}
	out := *g
	if g.Users != nil {
		out.Users = make([]User, len(g.Users))
		for i := range g.Users {
			out.Users[i] = *RedactUserCredentials(&g.Users[i])
		}
	}
	if g.Catalogues != nil {
		out.Catalogues = make([]Catalogue, len(g.Catalogues))
		for i, c := range g.Catalogues {
			c.LLMs = redactLLMs(c.LLMs)
			out.Catalogues[i] = c
		}
	}
	if g.DataCatalogues != nil {
		out.DataCatalogues = make([]DataCatalogue, len(g.DataCatalogues))
		for i, c := range g.DataCatalogues {
			c.Datasources = redactDatasources(c.Datasources)
			out.DataCatalogues[i] = c
		}
	}
	if g.ToolCatalogues != nil {
		out.ToolCatalogues = make([]ToolCatalogue, len(g.ToolCatalogues))
		for i, c := range g.ToolCatalogues {
			c.Tools = redactTools(c.Tools)
			out.ToolCatalogues[i] = c
		}
	}
	return &out
}

// RedactObjectCredentials is the copy of an LLM, tool, datasource, user,
// filter, App or group that may leave the process (events, object hooks);
// any other object is returned as it is.
func RedactObjectCredentials(object any) any {
	switch o := object.(type) {
	case *LLM:
		return RedactLLMCredentials(o)
	case LLM:
		return RedactLLMCredentials(&o)
	case *Tool:
		return RedactToolCredentials(o)
	case Tool:
		return RedactToolCredentials(&o)
	case *Datasource:
		return RedactDatasourceCredentials(o)
	case Datasource:
		return RedactDatasourceCredentials(&o)
	case *User:
		return RedactUserCredentials(o)
	case User:
		return RedactUserCredentials(&o)
	case *Filter:
		return RedactFilterCredentials(o)
	case Filter:
		return RedactFilterCredentials(&o)
	case *App:
		return RedactAppCredentials(o)
	case App:
		return RedactAppCredentials(&o)
	case *Group:
		return RedactGroupCredentials(o)
	case Group:
		return RedactGroupCredentials(&o)
	}
	return object
}

// RestoreObjectCredentials puts original's credentials back in modified (an
// object hook's result built from RedactObjectCredentials' copy) wherever it
// still reads CredentialRedacted. A datasource's embedder key needs nothing
// here: a redacted embed_api_key already keeps the stored key.
func RestoreObjectCredentials(original, modified any) {
	switch m := modified.(type) {
	case *LLM:
		if o, ok := original.(*LLM); ok {
			RestoreLLMCredentials(o, m)
			if o != nil && m != nil {
				restoreFiltersByID(o.Filters, m.Filters)
			}
		}
	case *Filter:
		if o, ok := original.(*Filter); ok {
			restoreFilterCredentials(o, m)
		}
	case *Tool:
		if o, ok := original.(*Tool); ok && o != nil && m != nil {
			restoreRedacted(&m.AuthKey, o.AuthKey)
			restoreFiltersByID(filterPtrs(o.Filters), filterPtrs(m.Filters))
			deps := make(map[uint]*Tool, len(o.Dependencies))
			for _, dep := range o.Dependencies {
				if dep != nil {
					deps[dep.ID] = dep
				}
			}
			for _, dep := range m.Dependencies {
				if dep != nil && deps[dep.ID] != nil {
					RestoreObjectCredentials(deps[dep.ID], dep)
				}
			}
		}
	case *Datasource:
		if o, ok := original.(*Datasource); ok && o != nil && m != nil {
			restoreRedacted(&m.DBConnString, o.DBConnString)
			restoreRedacted(&m.DBConnAPIKey, o.DBConnAPIKey)
		}
	case *User:
		if o, ok := original.(*User); ok && o != nil && m != nil {
			restoreRedacted(&m.Password, o.Password)
			restoreRedacted(&m.APIKey, o.APIKey)
			restoreRedacted(&m.SessionToken, o.SessionToken)
			restoreRedacted(&m.ResetToken, o.ResetToken)
			restoreRedacted(&m.VerificationToken, o.VerificationToken)
		}
	}
}
