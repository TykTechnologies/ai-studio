package models

import "encoding/json"

// Datasource JSON keeps the embed_vendor, embed_url, embed_api_key and
// embed_model keys it always had, now derived from the datasource's
// embedder. Object hooks, system events and submission version snapshots all
// serialise datasources this way, so plugins and stored versions see the same
// shape as before Embedders existed.
//
// On the way in, those keys are kept aside (LegacyEmbed) so a caller that
// edits them (a plugin hook, a version rollback) can have them resolved to an
// embedder; they never reach runtime code directly.

// LegacyEmbed is the inline embedding configuration a datasource JSON
// document carried.
type LegacyEmbed struct {
	Vendor string `json:"embed_vendor"`
	URL    string `json:"embed_url"`
	APIKey string `json:"embed_api_key"`
	Model  string `json:"embed_model"`
}

// RedactedLegacyKey replaces the embedder key in redacted JSON.
const RedactedLegacyKey = "[REDACTED]"

type datasourceAlias Datasource

// datasourceExtra is carried outside the gorm columns.
type datasourceExtra struct {
	legacyIn     *LegacyEmbed
	redactEmbKey bool
}

func (d *Datasource) extra() *datasourceExtra {
	if d.ext == nil {
		d.ext = &datasourceExtra{}
	}
	return d.ext
}

// FlattenedEmbed returns the datasource's embedder as the legacy inline
// fields, with references as stored (not resolved). A datasource without a
// resolvable embedder flattens to empty fields.
func (d *Datasource) FlattenedEmbed() LegacyEmbed {
	return d.EmbedFields(false)
}

// EmbedFields is FlattenedEmbed with the choice of resolving secret
// references (true for runtime and edge use). The datasource needs its
// Embedder (and that embedder's LLM) preloaded.
func (d *Datasource) EmbedFields(resolveSecrets bool) LegacyEmbed {
	if d.Embedder == nil {
		return LegacyEmbed{}
	}
	spec, err := d.Embedder.Spec(resolveSecrets)
	if err != nil {
		return LegacyEmbed{}
	}
	return LegacyEmbed{Vendor: string(spec.Vendor), URL: spec.Endpoint, APIKey: spec.APIKey, Model: spec.Model}
}

// RedactEmbedKey makes the datasource's JSON carry a redacted embedder key.
// It copies the JSON-only state first, so a redacted copy of a datasource
// (a struct copy shares the pointer) leaves the original untouched.
func (d *Datasource) RedactEmbedKey() {
	e := datasourceExtra{}
	if d.ext != nil {
		e = *d.ext
	}
	e.redactEmbKey = true
	d.ext = &e
}

// LegacyEmbedInput returns the embed_* keys this datasource was decoded
// from, if the JSON carried any.
func (d *Datasource) LegacyEmbedInput() (LegacyEmbed, bool) {
	if d.ext == nil || d.ext.legacyIn == nil {
		return LegacyEmbed{}, false
	}
	return *d.ext.legacyIn, true
}

// MarshalJSON adds the flattened embed_* keys.
func (d Datasource) MarshalJSON() ([]byte, error) {
	flat := d.FlattenedEmbed()
	if d.ext != nil && d.ext.redactEmbKey && flat.APIKey != "" {
		flat.APIKey = RedactedLegacyKey
	}
	return json.Marshal(struct {
		datasourceAlias
		LegacyEmbed
	}{datasourceAlias(d), flat})
}

// UnmarshalJSON reads the datasource and keeps any embed_* keys aside.
func (d *Datasource) UnmarshalJSON(data []byte) error {
	var a datasourceAlias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	*d = Datasource(a)
	d.ext = nil
	for _, k := range []string{"embed_vendor", "embed_url", "embed_api_key", "embed_model"} {
		if _, ok := probe[k]; ok {
			var le LegacyEmbed
			if err := json.Unmarshal(data, &le); err != nil {
				return err
			}
			d.extra().legacyIn = &le
			break
		}
	}
	return nil
}
