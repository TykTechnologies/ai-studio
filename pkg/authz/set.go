package authz

import "sort"

// Set is an effective permission set: either the wildcard, or a concrete set
// of permissions. It implements the implied-read rule: holding any action on
// a resource satisfies a read check for that resource.
type Set struct {
	all       bool
	perms     map[Permission]struct{}
	resources map[string]struct{}
}

// NewSet builds a concrete set. Sentinels are ignored except FullAdmin,
// which turns the set into the wildcard.
func NewSet(perms ...Permission) Set {
	s := Set{perms: map[Permission]struct{}{}, resources: map[string]struct{}{}}
	for _, p := range perms {
		s.add(p)
	}
	return s
}

// NewSetFromStrings builds a set from stored strings, skipping any that do
// not parse. Use this when loading role definitions from the database so a
// permission removed from the catalogue does not break evaluation.
func NewSetFromStrings(raw []string) Set {
	s := NewSet()
	for _, r := range raw {
		if p, err := Parse(r); err == nil {
			s.add(p)
		}
	}
	return s
}

// Wildcard returns the full-admin set.
func Wildcard() Set {
	return Set{all: true}
}

func (s *Set) add(p Permission) {
	if p == FullAdmin {
		s.all = true
		return
	}
	if p == AnyAdmin || p == "" {
		return
	}
	s.perms[p] = struct{}{}
	s.resources[p.Resource()] = struct{}{}
}

// Has reports whether the set satisfies p. FullAdmin is satisfied only by
// the wildcard. AnyAdmin is satisfied by any non-empty set.
func (s Set) Has(p Permission) bool {
	if s.all {
		return true
	}
	switch p {
	case FullAdmin:
		return false
	case AnyAdmin:
		return len(s.perms) > 0
	}
	if _, ok := s.perms[p]; ok {
		return true
	}
	if p.Action() == ActionRead {
		_, ok := s.resources[p.Resource()]
		return ok
	}
	return false
}

// HasAll reports whether every permission is satisfied.
func (s Set) HasAll(ps ...Permission) bool {
	for _, p := range ps {
		if !s.Has(p) {
			return false
		}
	}
	return true
}

// HasAny reports whether at least one permission is satisfied.
func (s Set) HasAny(ps ...Permission) bool {
	for _, p := range ps {
		if s.Has(p) {
			return true
		}
	}
	return false
}

// Union returns a new set containing both.
func (s Set) Union(o Set) Set {
	if s.all || o.all {
		return Wildcard()
	}
	out := NewSet()
	for p := range s.perms {
		out.add(p)
	}
	for p := range o.perms {
		out.add(p)
	}
	return out
}

// IsFullAdmin reports whether the set is the wildcard.
func (s Set) IsFullAdmin() bool { return s.all }

// IsEmpty reports whether the set grants nothing at all.
func (s Set) IsEmpty() bool { return !s.all && len(s.perms) == 0 }

// List returns the sorted permission strings; ["*"] for the wildcard.
func (s Set) List() []string {
	if s.all {
		return []string{string(FullAdmin)}
	}
	out := make([]string, 0, len(s.perms))
	for p := range s.perms {
		out = append(out, string(p))
	}
	sort.Strings(out)
	return out
}
