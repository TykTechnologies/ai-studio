package services

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/models"
	"github.com/TykTechnologies/midsommar/v2/secrets"
	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
)

// Secret names and the objects that reference them
// (midsommar-clean-oztk.47). A $SECRET/NAME reference resolves by name
// alone (secrets.GetSecretByVarName), so a name must keep meaning one
// secret, in the referencing object's Workspace:
//
//   - a name is unique among live secrets, across Workspaces (exact,
//     case-sensitive, as references match), checked on create and rename
//     under a per-name lock held in the same transaction as the write;
//   - a secret with referencing objects cannot be deleted or renamed;
//   - a new name cannot be taken while an object outside the new secret's
//     Workspace references it (a dangling reference would pick it up);
//   - a grandfathered duplicate name (from before this rule) cannot move.
//
// Plugin config references ($SECRET/ values in a plugin's or a per-LLM
// override's config) are not holders here: plugins are global (.36) and
// writing one needs global secrets:read, so they never tie a name to a
// Workspace and never block these writes.

// SecretHolder is an object whose credential field references a secret.
type SecretHolder struct {
	Type string `json:"type"`
	ID   uint   `json:"id"`
	Name string `json:"name"`
	// WorkspaceID is the holder's stored workspace_id (0 is the Default).
	WorkspaceID uint `json:"workspace_id"`
}

var (
	// ErrSecretNameTaken: another live secret has the name.
	ErrSecretNameTaken = errors.New("a secret with this name already exists")
	// ErrSecretDuplicateName: a grandfathered duplicate cannot move.
	ErrSecretDuplicateName = errors.New("another secret has the same name; rename one of them first")
)

// SecretReferencedError refuses deleting or renaming a secret that objects
// reference, or taking a name objects outside the secret's Workspace
// reference.
type SecretReferencedError struct {
	Holders []SecretHolder
	// Outside is true when the holders are outside the new secret's
	// Workspace (a create or rename onto a dangling name).
	Outside bool
}

func (e *SecretReferencedError) Error() string {
	if e.Outside {
		return fmt.Sprintf("%d object(s) in another Workspace reference this secret name; change those references first", len(e.Holders))
	}
	return fmt.Sprintf("the secret is referenced by %d object(s); change those references first", len(e.Holders))
}

// lockSecretName serializes writes that take or release name, until tx
// ends (Postgres: a transaction-level advisory lock on the name's hash,
// so it covers every replica; SQLite serializes writers).
func lockSecretName(tx *gorm.DB, name string) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "secret-name:"+name).Error
}

// SecretHoldersOf lists the live objects whose credential fields reference
// $SECRET/name: the recorded references (models.SecretReference: LLM key,
// endpoint and string metadata, tool auth key, datasource API key, embedder
// key and endpoint, which covers embedders created inline from a
// datasource) and, scanned directly because they are not recorded, a
// datasource's connection string and a guardrail filter's connection block.
func (s *Service) SecretHoldersOf(name string) ([]SecretHolder, error) {
	return secretHoldersOf(s.DB, name)
}

func secretHoldersOf(db *gorm.DB, name string) ([]SecretHolder, error) {
	ref := secrets.SecretRefPrefix + name
	var holders []SecretHolder
	seen := map[string]bool{}
	add := func(h SecretHolder) {
		key := fmt.Sprintf("%s/%d", h.Type, h.ID)
		if !seen[key] {
			seen[key] = true
			holders = append(holders, h)
		}
	}

	var recorded []models.SecretReference
	if db.Migrator().HasTable(&models.SecretReference{}) {
		if err := db.Where("secret_name = ?", name).Order("object_type, object_id").Find(&recorded).Error; err != nil {
			return nil, err
		}
	}
	byType := map[string][]uint{}
	for _, r := range recorded {
		byType[r.ObjectType] = append(byType[r.ObjectType], r.ObjectID)
	}
	tables := map[string]interface{}{
		models.SecretRefObjectLLM:        &models.LLM{},
		models.SecretRefObjectTool:       &models.Tool{},
		models.SecretRefObjectDatasource: &models.Datasource{},
		models.SecretRefObjectEmbedder:   &models.Embedder{},
	}
	for _, typ := range []string{models.SecretRefObjectLLM, models.SecretRefObjectTool, models.SecretRefObjectDatasource, models.SecretRefObjectEmbedder} {
		ids := byType[typ]
		if len(ids) == 0 {
			continue
		}
		var rows []struct {
			ID          uint
			Name        string
			WorkspaceID uint
		}
		if err := db.Model(tables[typ]).Select("id", "name", "workspace_id").Where("id IN ?", ids).Order("id").Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			add(SecretHolder{Type: typ, ID: r.ID, Name: r.Name, WorkspaceID: r.WorkspaceID})
		}
	}

	// Datasource connection strings (an indexed column; a reference is the
	// whole value). Tables are checked first: test databases that migrate
	// only some models still save secrets.
	var conns []struct {
		ID          uint
		Name        string
		WorkspaceID uint
	}
	if db.Migrator().HasTable(&models.Datasource{}) {
		if err := db.Model(&models.Datasource{}).Select("id", "name", "workspace_id").Where("db_conn_string = ?", ref).Order("id").Scan(&conns).Error; err != nil {
			return nil, err
		}
	}
	for _, r := range conns {
		add(SecretHolder{Type: models.SecretRefObjectDatasource, ID: r.ID, Name: r.Name, WorkspaceID: r.WorkspaceID})
	}

	// Guardrail filters: the connection block of the config. Filters are
	// few; the config is JSON, so it is read and matched here.
	var filters []models.Filter
	if db.Migrator().HasTable(&models.Filter{}) {
		if err := db.Select("id", "name", "workspace_id", "config").Where("kind = ?", models.FilterKindGuardrail).Order("id").Find(&filters).Error; err != nil {
			return nil, err
		}
	}
	for _, f := range filters {
		conn, _ := f.Config["connection"].(map[string]interface{})
		for _, v := range conn {
			if s, ok := v.(string); ok && s == ref {
				add(SecretHolder{Type: "filter", ID: f.ID, Name: f.Name, WorkspaceID: f.WorkspaceID})
				break
			}
		}
	}
	return holders, nil
}

// holdersOutside returns the holders whose Workspace is not ws, both read
// with 0 as the Default. It reads the Default through tx (a single-
// connection SQLite would otherwise wait on itself), and only when needed.
func (s *Service) holdersOutside(tx *gorm.DB, holders []SecretHolder, ws uint) ([]SecretHolder, error) {
	if len(holders) == 0 {
		return nil, nil
	}
	var def uint
	effective := func(id uint) (uint, error) {
		if id != 0 {
			return id, nil
		}
		if def == 0 {
			if cached := s.defaultWorkspaceID.Load(); cached != 0 {
				def = uint(cached)
			} else {
				w, err := models.GetDefaultWorkspace(tx)
				if err != nil {
					return 0, fmt.Errorf("load the default workspace: %w", err)
				}
				def = w.ID
			}
		}
		return def, nil
	}
	target, err := effective(ws)
	if err != nil {
		return nil, err
	}
	var out []SecretHolder
	for _, h := range holders {
		eff, err := effective(h.WorkspaceID)
		if err != nil {
			return nil, err
		}
		if eff != target {
			out = append(out, h)
		}
	}
	return out, nil
}

// claimSecretName checks, in tx under the name's lock, that name may be
// taken by the secret selfID (0 for a new one) in Workspace ws: no other
// live secret has it, and no object outside ws references it.
func (s *Service) claimSecretName(tx *gorm.DB, name string, selfID uint, ws uint) error {
	if err := lockSecretName(tx, name); err != nil {
		return err
	}
	var n int64
	q := tx.Model(&secrets.Secret{}).Where("var_name = ?", name)
	if selfID != 0 {
		q = q.Where("id <> ?", selfID)
	}
	if err := q.Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrSecretNameTaken
	}
	holders, err := secretHoldersOf(tx, name)
	if err != nil {
		return err
	}
	outside, err := s.holdersOutside(tx, holders, ws)
	if err != nil {
		return err
	}
	if len(outside) > 0 {
		return &SecretReferencedError{Holders: outside, Outside: true}
	}
	return nil
}

// releaseSecretName checks, in tx under the name's lock, that no object
// references name, before the secret holding it is deleted or renamed.
func releaseSecretName(tx *gorm.DB, name string) error {
	if err := lockSecretName(tx, name); err != nil {
		return err
	}
	holders, err := secretHoldersOf(tx, name)
	if err != nil {
		return err
	}
	if len(holders) > 0 {
		return &SecretReferencedError{Holders: holders}
	}
	return nil
}

// CreateSecret creates secret (secrets.CreateSecret: the value is
// encrypted) once its name is free (claimSecretName), in one transaction.
func (s *Service) CreateSecret(secret *secrets.Secret) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if err := s.claimSecretName(tx, secret.VarName, 0, secret.WorkspaceID); err != nil {
			return err
		}
		return secrets.CreateSecret(tx, secret)
	})
}

// UpdateSecret saves secret (secrets.UpdateSecret). When its name changes
// from oldName, the old name must be unreferenced and the new one free for
// a secret in Workspace ws (where it is after this update), in the same
// transaction (locks taken in name order).
func (s *Service) UpdateSecret(secret *secrets.Secret, oldName string, ws uint, encryptValue bool) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		if secret.VarName != oldName {
			first, second := oldName, secret.VarName
			if second < first {
				first, second = second, first
			}
			if err := lockSecretName(tx, first); err != nil {
				return err
			}
			if err := lockSecretName(tx, second); err != nil {
				return err
			}
			if err := releaseSecretName(tx, oldName); err != nil {
				return err
			}
			if err := s.claimSecretName(tx, secret.VarName, secret.ID, ws); err != nil {
				return err
			}
		}
		return secrets.UpdateSecret(tx, secret, encryptValue)
	})
}

// DeleteSecret deletes the secret with id once nothing references its name.
// A missing secret is not an error (as secrets.DeleteSecretByID).
func (s *Service) DeleteSecret(id uint) error {
	return s.DB.Transaction(func(tx *gorm.DB) error {
		var secret secrets.Secret
		err := tx.Select("id", "var_name").Where("id = ?", id).First(&secret).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := releaseSecretName(tx, secret.VarName); err != nil {
			return err
		}
		return secrets.DeleteSecretByID(tx, id)
	})
}

// RequireSecretMovable refuses (ErrSecretDuplicateName) moving a secret
// whose name another live secret also has (a grandfathered duplicate).
func (s *Service) RequireSecretMovable(secret *secrets.Secret) error {
	var n int64
	if err := s.DB.Model(&secrets.Secret{}).Where("var_name = ? AND id <> ?", secret.VarName, secret.ID).Count(&n).Error; err != nil {
		return err
	}
	if n > 0 {
		return ErrSecretDuplicateName
	}
	return nil
}
