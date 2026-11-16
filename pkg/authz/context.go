package authz

import (
	"sync"

	"github.com/gin-gonic/gin"
)

// ContextKey is the gin context key under which the request's Context is
// stored by the API's rbacContext middleware.
const ContextKey = "authz"

// Context carries the authenticated user's identity and a lazy resolver for
// their grants. Resolution runs at most once per request; nothing touches
// the database until something asks.
type Context struct {
	UserID  uint
	IsAdmin bool

	resolve func() (Grants, error)
	once    sync.Once
	grants  Grants
	err     error
}

// NewContext builds a Context for a user whose access is a global set only
// (Community Edition, tests).
func NewContext(userID uint, isAdmin bool, resolve func() (Set, error)) *Context {
	var r func() (Grants, error)
	if resolve != nil {
		r = func() (Grants, error) {
			s, err := resolve()
			return GlobalGrants(s), err
		}
	}
	return NewGrantsContext(userID, isAdmin, r)
}

// NewGrantsContext builds a Context with a resolver that returns global and
// per-Workspace grants.
func NewGrantsContext(userID uint, isAdmin bool, resolve func() (Grants, error)) *Context {
	return &Context{UserID: userID, IsAdmin: isAdmin, resolve: resolve}
}

// Grants resolves (once) and returns the user's grants.
func (x *Context) Grants() (Grants, error) {
	x.once.Do(func() {
		if x.resolve == nil {
			x.grants, x.err = GlobalGrants(NewSet()), nil
			return
		}
		x.grants, x.err = x.resolve()
	})
	return x.grants, x.err
}

// Permissions resolves (once) and returns the global set: the permissions
// that hold in every Workspace. Workspace-scoped grants are not included.
func (x *Context) Permissions() (Set, error) {
	g, err := x.Grants()
	return g.Global, err
}

// Can reports whether p is held globally. Any resolution error fails closed.
func (x *Context) Can(p Permission) bool {
	g, err := x.Grants()
	return err == nil && g.Can(p)
}

// CanIn reports whether p is held in the Workspace. Fails closed on error.
func (x *Context) CanIn(workspaceID uint, p Permission) bool {
	g, err := x.Grants()
	return err == nil && g.CanIn(workspaceID, p)
}

// CanAnywhere reports whether p is held globally or in any Workspace.
// Fails closed on error.
func (x *Context) CanAnywhere(p Permission) bool {
	g, err := x.Grants()
	return err == nil && g.CanAnywhere(p)
}

// WorkspacesWith reports where p is held; see Grants.WorkspacesWith. On a
// resolution error it reports nowhere.
func (x *Context) WorkspacesWith(p Permission) (bool, []uint) {
	g, err := x.Grants()
	if err != nil {
		return false, nil
	}
	return g.WorkspacesWith(p)
}

// WithContext stores x on the gin context.
func WithContext(c *gin.Context, x *Context) {
	c.Set(ContextKey, x)
}

// FromContext returns the request's Context, if the middleware set one.
func FromContext(c *gin.Context) (*Context, bool) {
	v, ok := c.Get(ContextKey)
	if !ok {
		return nil, false
	}
	x, ok := v.(*Context)
	return x, ok && x != nil
}

// AdminFlagged is implemented by the user model so this package can apply
// the legacy admin-or-not rule without importing it.
type AdminFlagged interface {
	AdminFlag() bool
}

// legacyAdmin applies the admin-or-not rule to a request without a Context.
func legacyAdmin(c *gin.Context) bool {
	if v, exists := c.Get("user"); exists {
		if u, ok := v.(AdminFlagged); ok && u.AdminFlag() {
			return true
		}
	}
	return false
}

// Can reports whether the request's user holds p globally. A
// Workspace-scoped grant never satisfies it. For a permission on a
// Workspace resource (ScopeOf(p).InWorkspace()) use CanIn with the object's Workspace, or
// CanAnywhere for a capability or read-global check; Can then answers only
// for global holders and refuses a user whose grant is scoped.
//
// With no Context on the request the route was registered outside the
// governed groups (or by a test that injects the user directly). The legacy
// rule then applies: a user flagged as administrator holds everything,
// anyone else nothing. Unauthenticated requests are always denied.
func Can(c *gin.Context, p Permission) bool {
	x, ok := FromContext(c)
	if ok {
		return x.Can(p)
	}
	return legacyAdmin(c)
}

// CanIn reports whether the request's user holds p in the Workspace. With
// no Context the admin-or-not rule applies, as for Can.
func CanIn(c *gin.Context, workspaceID uint, p Permission) bool {
	if x, ok := FromContext(c); ok {
		return x.CanIn(workspaceID, p)
	}
	return legacyAdmin(c)
}

// CanAnywhere reports whether the request's user holds p globally or in any
// Workspace. With no Context the admin-or-not rule applies, as for Can.
func CanAnywhere(c *gin.Context, p Permission) bool {
	if x, ok := FromContext(c); ok {
		return x.CanAnywhere(p)
	}
	return legacyAdmin(c)
}

// WorkspacesWith reports where the request's user holds p; see
// Grants.WorkspacesWith. With no Context an administrator holds it
// everywhere and anyone else nowhere.
func WorkspacesWith(c *gin.Context, p Permission) (all bool, ids []uint) {
	if x, ok := FromContext(c); ok {
		return x.WorkspacesWith(p)
	}
	return legacyAdmin(c), nil
}

// CanAny reports whether the request's user holds at least one of ps.
func CanAny(c *gin.Context, ps ...Permission) bool {
	for _, p := range ps {
		if Can(c, p) {
			return true
		}
	}
	return false
}

// Permissions returns the request user's global effective set. With no
// Context it returns an empty set and no error.
func Permissions(c *gin.Context) (Set, error) {
	x, ok := FromContext(c)
	if !ok {
		return NewSet(), nil
	}
	return x.Permissions()
}
