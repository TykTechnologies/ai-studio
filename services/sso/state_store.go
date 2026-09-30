package sso

import (
	"net/http"

	"github.com/gorilla/sessions"
)

// stateCookieMaxAge is gorilla/sessions' default cookie lifetime (30 days).
const stateCookieMaxAge = 86400 * 30

// NewStateStore returns the cookie store the embedded identity broker keeps
// its OAuth state in (tothic.Store, the _gothic_session cookie).
//
// The options are explicit because gorilla/sessions v1.4 changed
// NewCookieStore's defaults to Secure and SameSite=None: browsers drop such a
// cookie over plain HTTP, so social/OIDC/OAuth2 logins failed there with
// "could not find a matching session". The cookie is scoped to "/" (logout
// expires it there), HttpOnly, SameSite=Lax (the provider's callback is a
// top-level GET redirect, which Lax cookies accompany) and Secure exactly
// when secure is true, which callers take from the session cookie's setting.
func NewStateStore(secret []byte, secure bool) *sessions.CookieStore {
	store := sessions.NewCookieStore(secret)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   stateCookieMaxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
	store.MaxAge(stateCookieMaxAge)
	return store
}
