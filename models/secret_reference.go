package models

import (
	"strings"

	"gorm.io/gorm"
)

// SecretRefPrefix marks a credential field that reads a stored secret by
// name instead of carrying the value inline: "$SECRET/OPENAI_KEY".
const SecretRefPrefix = "$SECRET/"

// SecretReference records that one object reads a secret. It is the indexed,
// denormalised answer to "which LLMs, tools and datasources use this
// secret?", maintained by the AfterSave/AfterDelete hooks on those models so
// the secrets list and the per-secret dependents endpoint are single indexed
// queries rather than scans of the object tables.
//
// The rows are derived data: BackfillSecretReferences rebuilds them from the
// object tables at startup, which also repairs any drift from writes that
// bypassed the hooks (bulk SQL, external tooling).
type SecretReference struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	SecretName string `gorm:"size:255;index:idx_secret_references_name" json:"secret_name"`
	ObjectType string `gorm:"size:32;index:idx_secret_references_object,priority:1" json:"object_type"`
	ObjectID   uint   `gorm:"index:idx_secret_references_object,priority:2" json:"object_id"`
	ObjectName string `json:"object_name"`
}

// Object types recorded in SecretReference.ObjectType.
const (
	SecretRefObjectLLM        = "llm"
	SecretRefObjectTool       = "tool"
	SecretRefObjectDatasource = "datasource"
)

// SecretNameFromReference returns the secret name a $SECRET/<name> reference
// points at. Anything else (inline keys, $ENV/ references, empty) is not a
// secret reference.
func SecretNameFromReference(value string) (string, bool) {
	if !strings.HasPrefix(value, SecretRefPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(value, SecretRefPrefix)
	if name == "" {
		return "", false
	}
	return name, true
}

// secretNamesIn collects the distinct secret names referenced by the given
// values, in first-seen order.
func secretNamesIn(values []string) []string {
	var names []string
	seen := map[string]bool{}
	for _, v := range values {
		if name, ok := SecretNameFromReference(v); ok && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// secretReferenceIndexAvailable reports whether the secret_references table
// exists on this connection. The hooks are no-ops without it so a schema
// that has not (yet) been fully migrated, including the many tests that
// migrate only the models they need, can still save LLMs, tools and
// datasources; the startup backfill fills the index once it exists.
func secretReferenceIndexAvailable(tx *gorm.DB) bool {
	return tx.Session(&gorm.Session{NewDB: true}).Migrator().HasTable(&SecretReference{})
}

// syncSecretReferences replaces the recorded references of one object with
// the secrets named in values. It runs on the caller's connection so it
// takes part in the surrounding transaction.
func syncSecretReferences(tx *gorm.DB, objectType string, objectID uint, objectName string, values []string) error {
	if !secretReferenceIndexAvailable(tx) {
		return nil
	}
	db := tx.Session(&gorm.Session{NewDB: true})
	if err := db.Where("object_type = ? AND object_id = ?", objectType, objectID).
		Delete(&SecretReference{}).Error; err != nil {
		return err
	}
	names := secretNamesIn(values)
	if len(names) == 0 {
		return nil
	}
	rows := make([]SecretReference, 0, len(names))
	for _, name := range names {
		rows = append(rows, SecretReference{SecretName: name, ObjectType: objectType, ObjectID: objectID, ObjectName: objectName})
	}
	return db.Create(&rows).Error
}

// clearSecretReferences drops every reference recorded for one object.
func clearSecretReferences(tx *gorm.DB, objectType string, objectID uint) error {
	if !secretReferenceIndexAvailable(tx) {
		return nil
	}
	return tx.Session(&gorm.Session{NewDB: true}).
		Where("object_type = ? AND object_id = ?", objectType, objectID).
		Delete(&SecretReference{}).Error
}

// llmSecretValues lists every LLM field that may hold a $SECRET/ reference:
// the key, the endpoint, and string values in the vendor metadata map
// (Bedrock keeps its AWS keys there).
func llmSecretValues(l *LLM) []string {
	values := []string{l.APIKey, l.APIEndpoint}
	for _, v := range l.Metadata {
		if s, ok := v.(string); ok {
			values = append(values, s)
		}
	}
	return values
}

// AfterSave keeps the secret_references rows for this LLM current. The row
// is re-read because a partial update (Model(&llm).Update("active", ...))
// reaches the hook with only the fields the caller set.
func (l *LLM) AfterSave(tx *gorm.DB) error {
	if l.ID == 0 {
		return nil
	}
	if !secretReferenceIndexAvailable(tx) {
		return nil
	}
	var fresh LLM
	if err := tx.Session(&gorm.Session{NewDB: true}).
		Select("id", "name", "api_key", "api_endpoint", "metadata").
		First(&fresh, l.ID).Error; err != nil {
		return err
	}
	return syncSecretReferences(tx, SecretRefObjectLLM, fresh.ID, fresh.Name, llmSecretValues(&fresh))
}

// AfterDelete drops the LLM's secret references (soft deletes included).
func (l *LLM) AfterDelete(tx *gorm.DB) error {
	if l.ID == 0 {
		return nil
	}
	return clearSecretReferences(tx, SecretRefObjectLLM, l.ID)
}

// AfterSave keeps the secret_references rows for this tool current.
func (t *Tool) AfterSave(tx *gorm.DB) error {
	if t.ID == 0 {
		return nil
	}
	if !secretReferenceIndexAvailable(tx) {
		return nil
	}
	var fresh Tool
	if err := tx.Session(&gorm.Session{NewDB: true}).
		Select("id", "name", "auth_key").
		First(&fresh, t.ID).Error; err != nil {
		return err
	}
	return syncSecretReferences(tx, SecretRefObjectTool, fresh.ID, fresh.Name, []string{fresh.AuthKey})
}

// AfterDelete drops the tool's secret references.
func (t *Tool) AfterDelete(tx *gorm.DB) error {
	if t.ID == 0 {
		return nil
	}
	return clearSecretReferences(tx, SecretRefObjectTool, t.ID)
}

// AfterSave keeps the secret_references rows for this datasource current.
func (d *Datasource) AfterSave(tx *gorm.DB) error {
	if d.ID == 0 {
		return nil
	}
	if !secretReferenceIndexAvailable(tx) {
		return nil
	}
	var fresh Datasource
	if err := tx.Session(&gorm.Session{NewDB: true}).
		Select("id", "name", "db_conn_api_key", "embed_api_key").
		First(&fresh, d.ID).Error; err != nil {
		return err
	}
	return syncSecretReferences(tx, SecretRefObjectDatasource, fresh.ID, fresh.Name, []string{fresh.DBConnAPIKey, fresh.EmbedAPIKey})
}

// AfterDelete drops the datasource's secret references.
func (d *Datasource) AfterDelete(tx *gorm.DB) error {
	if d.ID == 0 {
		return nil
	}
	return clearSecretReferences(tx, SecretRefObjectDatasource, d.ID)
}

// BackfillSecretReferences rebuilds secret_references from the object tables.
// It runs once at startup: the first boot after this table was introduced
// populates it, and every later boot repairs drift from writes that bypassed
// the model hooks. Only rows whose credential columns can hold a reference
// are loaded (prefix match on the plain columns, a contains match on the
// JSON metadata), so the scan stays proportional to secret-backed objects.
func BackfillSecretReferences(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&SecretReference{}).Error; err != nil {
			return err
		}

		prefix := likeEscape(SecretRefPrefix) + "%"
		contains := "%" + likeEscape(SecretRefPrefix) + "%"
		const esc = " ESCAPE '\\'"

		var llms []LLM
		if err := tx.Select("id", "name", "api_key", "api_endpoint", "metadata").
			Where("api_key LIKE ?"+esc+" OR api_endpoint LIKE ?"+esc+" OR CAST(metadata AS TEXT) LIKE ?"+esc, prefix, prefix, contains).
			Find(&llms).Error; err != nil {
			return err
		}
		for i := range llms {
			if err := syncSecretReferences(tx, SecretRefObjectLLM, llms[i].ID, llms[i].Name, llmSecretValues(&llms[i])); err != nil {
				return err
			}
		}

		var tools []Tool
		if err := tx.Select("id", "name", "auth_key").
			Where("auth_key LIKE ?"+esc, prefix).
			Find(&tools).Error; err != nil {
			return err
		}
		for _, t := range tools {
			if err := syncSecretReferences(tx, SecretRefObjectTool, t.ID, t.Name, []string{t.AuthKey}); err != nil {
				return err
			}
		}

		var datasources []Datasource
		if err := tx.Select("id", "name", "db_conn_api_key", "embed_api_key").
			Where("db_conn_api_key LIKE ?"+esc+" OR embed_api_key LIKE ?"+esc, prefix, prefix).
			Find(&datasources).Error; err != nil {
			return err
		}
		for _, d := range datasources {
			if err := syncSecretReferences(tx, SecretRefObjectDatasource, d.ID, d.Name, []string{d.DBConnAPIKey, d.EmbedAPIKey}); err != nil {
				return err
			}
		}
		return nil
	})
}

// likeEscape escapes the LIKE wildcards (and the escape character itself) in
// a literal so it can be embedded in a pattern that runs with ESCAPE '\'.
func likeEscape(literal string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(literal)
}
