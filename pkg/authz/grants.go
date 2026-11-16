package authz

import "sort"

// Grants is a user's resolved access: the permissions held globally (from
// unscoped bindings, covering every Workspace) and, per Workspace id, the
// permissions held only inside that Workspace (from Workspace-scoped
// bindings).
//
// A Workspace set only ever counts for permissions on Workspace resources
// (ScopeOf(p).InWorkspace(): stamped ones, and credentials through their
// App). Every method below enforces that, whatever the
// sets contain, so a global permission (users, roles, edges, plugins,
// webhooks, plugin-contributed resources, the sentinels) can never be
// satisfied by a scoped binding. That also confines the plugins:execute
// umbrella rule of Set.Has to Global.
type Grants struct {
	Global     Set
	Workspaces map[uint]Set
}

// GlobalGrants wraps a global set with no Workspace grants: what Community
// Edition and an unlicensed Enterprise build resolve.
func GlobalGrants(s Set) Grants {
	return Grants{Global: s}
}

// Can reports whether p is held globally. Use it for global resources and
// wherever a check must not be satisfied by a Workspace grant.
func (g Grants) Can(p Permission) bool {
	return g.Global.Has(p)
}

// CanIn reports whether p is held in the given Workspace: globally, or by a
// binding scoped to that Workspace when p is on a Workspace resource. For a
// ScopeWorkspaceParent resource pass the parent object's Workspace.
// Workspace id 0 never matches a Workspace set: callers map a stored
// workspace_id of 0 to the Default workspace first (EffectiveWorkspaceID).
func (g Grants) CanIn(workspaceID uint, p Permission) bool {
	if g.Global.Has(p) {
		return true
	}
	if workspaceID == 0 || !ScopeOf(p).InWorkspace() {
		return false
	}
	s, ok := g.Workspaces[workspaceID]
	return ok && s.Has(p)
}

// CanAnywhere reports whether p is held globally or in at least one
// Workspace. It is the route-level capability check for stamped resources
// (the object check comes later) and the read rule for read-global
// resources (decision D4). For AnyAdmin it reports whether the user holds
// anything at all, in any scope.
func (g Grants) CanAnywhere(p Permission) bool {
	if g.Global.Has(p) {
		return true
	}
	if p == AnyAdmin {
		for _, s := range g.Workspaces {
			if !s.IsEmpty() {
				return true
			}
		}
		return false
	}
	if !ScopeOf(p).InWorkspace() {
		return false
	}
	for id, s := range g.Workspaces {
		if id != 0 && s.Has(p) {
			return true
		}
	}
	return false
}

// WorkspacesWith reports where p is held: all is true when it is held
// globally (every Workspace); otherwise ids lists, sorted, the Workspaces
// whose scoped grants satisfy it. List filters use it.
func (g Grants) WorkspacesWith(p Permission) (all bool, ids []uint) {
	if g.Global.Has(p) {
		return true, nil
	}
	if !ScopeOf(p).InWorkspace() {
		return false, nil
	}
	for id, s := range g.Workspaces {
		if id != 0 && s.Has(p) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return false, ids
}

// NewScopedSetFromStrings builds the set a Workspace-scoped binding of a
// role grants (decision D2): permissions on global resources are dropped,
// and the wildcard expands to every action of every Workspace resource
// (WorkspaceResources), never to the full-admin wildcard. Unknown strings are skipped, as in
// NewSetFromStrings.
func NewScopedSetFromStrings(raw []string) Set {
	s := NewSet()
	for _, r := range raw {
		p, err := Parse(r)
		if err != nil {
			continue
		}
		if p == FullAdmin {
			for _, key := range WorkspaceResources() {
				if res, ok := ResourceByKey(key); ok {
					for _, a := range res.Actions {
						s.add(P(key, a))
					}
				}
			}
			continue
		}
		if ScopeOf(p).InWorkspace() {
			s.add(p)
		}
	}
	return s
}
