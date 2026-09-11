package authz

import (
	"sync"

	"github.com/gin-gonic/gin"
)

// ContextKey is the gin context key under which the request's Context is
// stored by the API's rbacContext middleware.
const ContextKey = "authz"

// Context carries the authenticated user's identity and a lazy resolver for
// their effective permission set. Resolution runs at most once per request;
// nothing touches the database until something asks.
type Context struct {
	UserID  uint
	IsAdmin bool

	resolve func() (Set, error)
	once    sync.Once
	set     Set
	err     error
}

// NewContext builds a Context for a user with the given resolver.
func NewContext(userID uint, isAdmin bool, resolve func() (Set, error)) *Context {
	return &Context{UserID: userID, IsAdmin: isAdmin, resolve: resolve}
}

// Permissions resolves (once) and returns the effective set.
func (x *Context) Permissions() (Set, error) {
	x.once.Do(func() {
		if x.resolve == nil {
			x.set, x.err = NewSet(), nil
			return
		}
		x.set, x.err = x.resolve()
	})
	return x.set, x.err
}

// Can reports whether the set satisfies p. Any resolution error fails closed.
func (x *Context) Can(p Permission) bool {
	set, err := x.Permissions()
	if err != nil {
		return false
	}
	return set.Has(p)
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

// Can reports whether the request's user holds p.
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
	if v, exists := c.Get("user"); exists {
		if u, ok := v.(AdminFlagged); ok && u.AdminFlag() {
			return true
		}
	}
	return false
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

// Permissions returns the request user's effective set. With no Context it
// returns an empty set and no error.
func Permissions(c *gin.Context) (Set, error) {
	x, ok := FromContext(c)
	if !ok {
		return NewSet(), nil
	}
	return x.Permissions()
}
