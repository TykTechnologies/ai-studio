package sso

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stateCookie saves a session through store and returns the Set-Cookie
// header it writes for the broker's _gothic_session cookie.
func stateCookie(t *testing.T, secure bool) (string, *http.Cookie) {
	t.Helper()
	store := NewStateStore([]byte("0123456789abcdef0123456789abcdef"), secure)
	req := httptest.NewRequest(http.MethodGet, "http://studio.internal:8080/auth/github", nil)
	rec := httptest.NewRecorder()
	session, err := store.New(req, "_gothic_session")
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	session.Values["github"] = "state"
	if err := session.Save(req, rec); err != nil {
		t.Fatalf("save session: %v", err)
	}
	headers := rec.Result().Header.Values("Set-Cookie")
	if len(headers) != 1 {
		t.Fatalf("want one Set-Cookie header, got %q", headers)
	}
	cookies := rec.Result().Cookies()
	return headers[0], cookies[0]
}

// Over plain HTTP (a DEVMODE install on an IP or hostname) the state cookie
// must be one the browser keeps: not Secure and not SameSite=None, which
// gorilla/sessions v1.4 made the default (H2 in the post-2.2 sweep).
func TestNewStateStore_PlainHTTP(t *testing.T) {
	header, c := stateCookie(t, false)
	if c.Secure {
		t.Errorf("state cookie must not be Secure over plain HTTP: %s", header)
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("want SameSite=Lax, got %v: %s", c.SameSite, header)
	}
	if strings.Contains(header, "SameSite=None") {
		t.Errorf("SameSite=None needs Secure; browsers drop it over HTTP: %s", header)
	}
	if !c.HttpOnly || c.Path != "/" || c.MaxAge != 86400*30 {
		t.Errorf("want HttpOnly, Path=/ and a 30-day Max-Age: %s", header)
	}
}

func TestNewStateStore_HTTPS(t *testing.T) {
	header, c := stateCookie(t, true)
	if !c.Secure {
		t.Errorf("state cookie must be Secure over HTTPS: %s", header)
	}
	if c.SameSite != http.SameSiteLaxMode || !c.HttpOnly || c.Path != "/" {
		t.Errorf("want SameSite=Lax, HttpOnly, Path=/: %s", header)
	}
}

// The cookie the store writes is one it reads back, so the callback finds
// the state the login step stored.
func TestNewStateStore_RoundTrip(t *testing.T) {
	store := NewStateStore([]byte("0123456789abcdef0123456789abcdef"), false)
	_, c := stateCookie(t, false)
	req := httptest.NewRequest(http.MethodGet, "http://studio.internal:8080/auth/github/callback", nil)
	req.AddCookie(c)
	session, err := store.Get(req, "_gothic_session")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.IsNew || session.Values["github"] != "state" {
		t.Fatalf("state not read back: new=%v values=%v", session.IsNew, session.Values)
	}
}
