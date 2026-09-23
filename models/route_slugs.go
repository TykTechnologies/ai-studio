package models

import (
	"errors"
	"fmt"

	"github.com/gosimple/slug"
	"gorm.io/gorm"
)

// Route slugs.
//
// The gateway's /ai/{route} chain, and the unified ingress in front of it
// ({"model": "{route}/{model}"}), resolve one namespace of route names: an
// LLM answers to slug.Make(its name), a Model Router to its slug. An LLM wins
// a clash, so a router whose slug an LLM also answers to could never be
// reached. The two are kept apart when either is saved. The check ignores
// namespaces: an edge serves its own namespace and the global one, so two
// objects that share a slug across namespaces can still meet on one edge.

// ErrRouteSlugTaken is returned when a name or slug would clash with another
// route on the gateway.
var ErrRouteSlugTaken = errors.New("route name is already used by another LLM or router")

// LLMRouteSlug is the route an LLM answers to on the gateway.
func LLMRouteSlug(name string) string { return slug.Make(name) }

// CheckLLMRouteSlug refuses an LLM name whose route a Model Router already
// uses.
func CheckLLMRouteSlug(db *gorm.DB, name string) error {
	s := LLMRouteSlug(name)
	var count int64
	if err := db.Model(&ModelRouter{}).Where("slug = ?", s).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: a Model Router uses %q", ErrRouteSlugTaken, s)
	}
	return nil
}

// CheckRouterRouteSlug refuses a router slug an LLM already answers to.
func CheckRouterRouteSlug(db *gorm.DB, routerSlug string) error {
	var names []string
	if err := db.Model(&LLM{}).Pluck("name", &names).Error; err != nil {
		return err
	}
	for _, n := range names {
		if LLMRouteSlug(n) == routerSlug {
			return fmt.Errorf("%w: the LLM %q answers to %q", ErrRouteSlugTaken, n, routerSlug)
		}
	}
	return nil
}
