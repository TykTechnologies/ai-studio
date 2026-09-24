package models

import (
	"fmt"
	"log"
	"strings"

	sr "github.com/TykTechnologies/midsommar/v2/pkg/semanticrouting"
	"gorm.io/gorm"
)

// embedderMigrationLockKey is the Postgres advisory lock that serialises
// MigrateEmbedders across Studio replicas starting together.
const embedderMigrationLockKey = 7302194511

// legacyEmbedColumns are the datasource columns Embedders replaced. They stay
// in the table (cleared) until a later release drops them.
var legacyEmbedColumns = []string{"embed_vendor", "embed_url", "embed_api_key", "embed_model"}

type legacyEmbedRow struct {
	ID           uint
	Name         string
	PrivacyScore int
	DBConnString string
	DBConnAPIKey string
	EmbedVendor  string
	EmbedURL     string `gorm:"column:embed_url"`
	EmbedAPIKey  string `gorm:"column:embed_api_key"`
	EmbedModel   string
}

type legacyEmbedKey struct {
	vendor, endpoint, key, model string
}

// effective is the embedding configuration the row actually used. The Vertex
// embedder read its project:location and key from the vector store columns,
// so a Vertex row without its own endpoint keeps working that way.
func (r legacyEmbedRow) effective() legacyEmbedKey {
	k := legacyEmbedKey{vendor: r.EmbedVendor, endpoint: r.EmbedURL, key: r.EmbedAPIKey, model: r.EmbedModel}
	if Vendor(r.EmbedVendor) == VERTEX && k.endpoint == "" {
		k.endpoint = r.DBConnString
		if k.key == "" {
			k.key = r.DBConnAPIKey
		}
	}
	return k
}

// MigrateEmbedders moves datasources' inline embedding settings onto Embedder
// rows: identical settings (vendor, endpoint, key, model) share one embedder,
// each datasource is linked to its embedder and its inline columns are
// cleared. Keys are plain values or $SECRET/ / $ENV/ references (never
// ciphertext), so comparing them as strings is exact. A shared embedder takes
// the highest privacy score of its datasources, so none of them fails the
// embedder privacy check afterwards.
//
// It selects only datasources that still carry settings and no embedder, so
// once it has run it finds nothing. On Postgres an advisory lock keeps two
// replicas from migrating the same rows.
func MigrateEmbedders(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if tx.Dialector.Name() == "postgres" {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", embedderMigrationLockKey).Error; err != nil {
				return err
			}
		}
		if err := migrateRouterEmbedders(tx); err != nil {
			return err
		}
		m := tx.Migrator()
		for _, c := range legacyEmbedColumns {
			if !m.HasColumn("datasources", c) {
				return nil // a database created after the columns went away
			}
		}

		var rows []legacyEmbedRow
		if err := tx.Table("datasources").
			Select("id, name, privacy_score, db_conn_string, db_conn_api_key, embed_vendor, embed_url, embed_api_key, embed_model").
			Where("embedder_id IS NULL AND COALESCE(embed_vendor, '') <> '' AND deleted_at IS NULL").
			Order("id").
			Scan(&rows).Error; err != nil {
			return err
		}
		if err := clearStaleEmbedColumns(tx); err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}

		var order []legacyEmbedKey
		groups := map[legacyEmbedKey][]legacyEmbedRow{}
		for _, r := range rows {
			k := r.effective()
			if _, ok := groups[k]; !ok {
				order = append(order, k)
			}
			groups[k] = append(groups[k], r)
		}

		for _, k := range order {
			members := groups[k]
			if strings.TrimSpace(k.model) == "" {
				log.Printf("embedder migration: datasource %q (id %d) has embedding vendor %q but no model; its embedder needs a model before it can embed",
					members[0].Name, members[0].ID, k.vendor)
			}
			privacy := 0
			ids := make([]uint, 0, len(members))
			for _, r := range members {
				if r.PrivacyScore > privacy {
					privacy = r.PrivacyScore
				}
				ids = append(ids, r.ID)
			}
			name, err := uniqueEmbedderName(tx, defaultEmbedderName(k.vendor, k.model))
			if err != nil {
				return err
			}
			e := &Embedder{
				Name:         name,
				Description:  migratedDescription(members),
				Vendor:       Vendor(k.vendor),
				Endpoint:     k.endpoint,
				APIKey:       k.key,
				ModelName:    k.model,
				PrivacyScore: privacy,
			}
			// Create directly: a datasource with a vendor but no model must
			// still be linked, even though Validate would reject the row.
			if err := tx.Omit("LLM").Create(e).Error; err != nil {
				return fmt.Errorf("embedder migration: create %q: %w", name, err)
			}
			if err := tx.Table("datasources").Where("id IN ?", ids).Updates(map[string]interface{}{
				"embedder_id":   e.ID,
				"embed_vendor":  "",
				"embed_url":     "",
				"embed_api_key": "",
				"embed_model":   "",
			}).Error; err != nil {
				return err
			}
		}
		log.Printf("embedder migration: moved %d datasource(s) onto %d embedder(s)", len(rows), len(order))
		return nil
	})
}

// clearStaleEmbedColumns empties the retired columns on rows the migration
// does not move: soft-deleted datasources, and settings without a vendor
// (never usable). Either way their keys would otherwise stay in columns
// nothing reads, outside the secret reference index.
func clearStaleEmbedColumns(tx *gorm.DB) error {
	res := tx.Table("datasources").
		Where("embedder_id IS NULL AND (deleted_at IS NOT NULL OR COALESCE(embed_vendor, '') = '')").
		Where("COALESCE(embed_vendor, '') <> '' OR COALESCE(embed_url, '') <> '' OR COALESCE(embed_api_key, '') <> '' OR COALESCE(embed_model, '') <> ''").
		Updates(map[string]interface{}{"embed_vendor": "", "embed_url": "", "embed_api_key": "", "embed_model": ""})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		log.Printf("embedder migration: cleared embedding settings on %d deleted or vendorless datasource(s)", res.RowsAffected)
	}
	return nil
}

// migrateRouterEmbedders moves Semantic Routers whose embedding stage names
// an LLM ({llm_id, model} in settings) onto the embedder linked to that LLM
// with that model, keeping only the timeout in settings. A router whose LLM
// is gone is left as it is (and logged): it could not embed either way.
func migrateRouterEmbedders(tx *gorm.DB) error {
	var routers []SemanticRouter
	if err := tx.Where("embedder_id IS NULL").Find(&routers).Error; err != nil {
		return err
	}
	moved := 0
	for i := range routers {
		r := &routers[i]
		ref := r.Settings.Embedding
		if ref == nil || ref.LLMID == 0 {
			continue
		}
		e, _, err := FindOrCreateLinkedEmbedder(tx, ref.LLMID, ref.Model, 0)
		if err != nil {
			log.Printf("embedder migration: semantic router %q (id %d) embeds with LLM %d, which cannot be linked: %v", r.Name, r.ID, ref.LLMID, err)
			continue
		}
		id := e.ID
		r.EmbedderID = &id
		r.Embedder = e
		if ref.TimeoutMs > 0 {
			r.Settings.Embedding = &sr.ModelRef{TimeoutMs: ref.TimeoutMs}
		} else {
			r.Settings.Embedding = nil
		}
		if err := tx.Model(r).Select("EmbedderID", "Settings").Updates(r).Error; err != nil {
			return err
		}
		if err := r.writeTargets(tx); err != nil {
			return err
		}
		moved++
	}
	if moved > 0 {
		log.Printf("embedder migration: moved %d semantic router(s) onto linked embedders", moved)
	}
	return nil
}

// defaultEmbedderName is the name an embedder gets when one is created for
// the caller: "<vendor> · <model>".
func defaultEmbedderName(vendor, model string) string {
	if model == "" {
		model = "no model"
	}
	return fmt.Sprintf("%s · %s", vendor, model)
}

// uniqueEmbedderName returns base, or base with " (n)" appended when the
// name is taken.
func uniqueEmbedderName(tx *gorm.DB, base string) (string, error) {
	name := base
	for n := 2; ; n++ {
		var count int64
		if err := tx.Model(&Embedder{}).Unscoped().Where("name = ?", name).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return name, nil
		}
		name = fmt.Sprintf("%s (%d)", base, n)
	}
}

// UniqueEmbedderName is uniqueEmbedderName for the service layer.
func UniqueEmbedderName(tx *gorm.DB, vendor, model string) (string, error) {
	return uniqueEmbedderName(tx, defaultEmbedderName(vendor, model))
}

func migratedDescription(members []legacyEmbedRow) string {
	names := make([]string, 0, len(members))
	for _, r := range members {
		names = append(names, r.Name)
	}
	return "Created from the embedding settings of: " + strings.Join(names, ", ")
}
