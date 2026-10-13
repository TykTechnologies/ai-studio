package models

import (
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/third_party/gorm.io/gorm"
	"github.com/gosimple/slug"
)

// ErrSlugConflict matches (errors.Is) every *SlugConflictError.
var ErrSlugConflict = errors.New("slug already in use")

// SlugConflictError refuses a tool or LLM name whose slug another one
// already has. The slug (lower case, with spaces and punctuation folded to
// "-") is how gateways address tools and LLMs, and edge gateways store it
// under a unique index.
//
// The checks ignore namespaces, like the route slug checks (route_slugs.go):
// an edge gets the global namespace and its own, and an object can move
// between namespaces.
type SlugConflictError struct {
	Kind         string // "tool" or "LLM"
	Name         string
	Slug         string
	ExistingID   uint
	ExistingName string
}

func (e *SlugConflictError) Error() string {
	return fmt.Sprintf("%s name %q is too close to %s %d (%q): both have the slug %q, and %s names must differ by more than case, spaces or punctuation",
		e.Kind, e.Name, e.Kind, e.ExistingID, e.ExistingName, e.Slug, e.Kind)
}

// Is matches ErrSlugConflict, and for an LLM also ErrRouteSlugTaken: an
// LLM's slug is its route on the gateway.
func (e *SlugConflictError) Is(target error) bool {
	return target == ErrSlugConflict || (e.Kind == "LLM" && target == ErrRouteSlugTaken)
}

// hookDB is a fresh session on the hook's connection (its transaction, if
// any) for the queries a hook runs.
func hookDB(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true})
}

// checkToolSlug refuses t's slug if another tool has it. A tool whose slug
// is unchanged is not checked, so a duplicate stored before this check can
// still be edited (and renamed).
func checkToolSlug(db *gorm.DB, t *Tool) error {
	if t.ID != 0 {
		var stored Tool
		err := db.Model(&Tool{}).Select("id", "slug").Where("id = ?", t.ID).Take(&stored).Error
		if err == nil && stored.Slug == t.Slug {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	return toolSlugFree(db, t.ID, t.Name, t.Slug)
}

func toolSlugFree(db *gorm.DB, id uint, name, toolSlug string) error {
	var other Tool
	q := db.Model(&Tool{}).Select("id", "name").Where("slug = ?", toolSlug)
	if id != 0 {
		q = q.Where("id <> ?", id)
	}
	err := q.Order("id").Take(&other).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return &SlugConflictError{Kind: "tool", Name: name, Slug: toolSlug, ExistingID: other.ID, ExistingName: other.Name}
}

// PrepareToolUpdates readies a column update of tool id (an Updates map,
// which runs no slug hook): a new name brings its slug along, and a name
// whose slug another tool has is refused.
func PrepareToolUpdates(db *gorm.DB, id uint, updates map[string]interface{}) error {
	name, renamed := updates["name"].(string)
	if !renamed {
		return nil
	}
	toolSlug := slug.Make(name)
	updates["slug"] = toolSlug
	var stored Tool
	if err := db.Model(&Tool{}).Select("id", "slug").Where("id = ?", id).Take(&stored).Error; err != nil {
		return err
	}
	if toolSlug == stored.Slug {
		return nil
	}
	return toolSlugFree(db, id, name, toolSlug)
}

// checkLLMSlug refuses l's name if another LLM has the same slug (LLMs store
// no slug: it is derived from the name wherever it is used, see
// LLMRouteSlug). An LLM whose slug is unchanged is not checked.
func checkLLMSlug(db *gorm.DB, l *LLM) error {
	llmSlug := LLMRouteSlug(l.Name)
	if l.ID != 0 {
		var stored LLM
		err := db.Model(&LLM{}).Select("id", "name").Where("id = ?", l.ID).Take(&stored).Error
		if err == nil && LLMRouteSlug(stored.Name) == llmSlug {
			return nil
		}
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	var others []LLM
	q := db.Model(&LLM{}).Select("id", "name")
	if l.ID != 0 {
		q = q.Where("id <> ?", l.ID)
	}
	if err := q.Order("id").Find(&others).Error; err != nil {
		return err
	}
	for _, o := range others {
		if LLMRouteSlug(o.Name) == llmSlug {
			return &SlugConflictError{Kind: "LLM", Name: l.Name, Slug: llmSlug, ExistingID: o.ID, ExistingName: o.Name}
		}
	}
	return nil
}
