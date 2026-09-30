package services

import (
	"net/http"
	"testing"

	"github.com/TykTechnologies/tyk-identity-broker/tothic"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
)

// InitInternalTIB gives the broker a state store whose cookie a browser
// keeps over plain HTTP, and a Secure one when the session cookie is Secure.
func TestInitInternalTIB_StateCookieFollowsCookieSecure(t *testing.T) {
	prev := tothic.Store
	t.Cleanup(func() { tothic.Store = prev })

	for _, secure := range []bool{false, true} {
		s := NewSSOService(&Config{APISecret: "test-secret", LogLevel: "info", CookieSecure: secure}, gin.New(), setupSSOTestDB(t), nil)
		s.InitInternalTIB()

		store, ok := tothic.Store.(*sessions.CookieStore)
		if !ok {
			t.Fatalf("tothic.Store is %T, want *sessions.CookieStore", tothic.Store)
		}
		o := store.Options
		if o.Secure != secure || o.SameSite != http.SameSiteLaxMode || !o.HttpOnly || o.Path != "/" {
			t.Errorf("CookieSecure=%v: got options %+v", secure, *o)
		}
	}
}
