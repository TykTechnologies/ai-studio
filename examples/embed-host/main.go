// Command embed-host is a minimal application that embeds AI Studio: it
// signs users in with its own (deliberately trivial) login page, serves
// Studio under /ai-studio, and lets Studio provision and authorise the
// users it vouches for. It exists to show the pkg/studio wiring and to try
// embedded mode by hand; see pkg/studio/README.md.
//
//	go run ./examples/embed-host -addr :8090
//	TYK_AI_LICENSE=... go run -tags enterprise ./examples/embed-host   # Enterprise Edition
//
// Then open http://localhost:8090/, sign in as "admin" (a Studio
// administrator) or any other name (a regular user), and follow the link
// into Studio. The frontend must be built first (npm run build in
// ui/admin-frontend), as for the standalone binary; or build with
// -tags studio_noui and pass -ui with the unpacked tyk-ai-studio-ui release
// tarball, as a host importing Studio as a module does.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/TykTechnologies/midsommar/v2/config"
	"github.com/TykTechnologies/midsommar/v2/pkg/studio"
	// This demo keeps Studio in a SQLite file; a Postgres-only host skips it.
	_ "github.com/TykTechnologies/midsommar/v2/pkg/studio/sqlitedb"
)

const (
	basePath   = "/ai-studio"
	hostCookie = "host_user"
)

var validName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// licenseSource supplies the Enterprise licence (main_enterprise.go); nil in
// the Community Edition.
var licenseSource func() string

// cookieAuth is the host's authenticator: whoever the host_user cookie names
// is signed in. A real host would check its own session here.
type cookieAuth struct{}

func (cookieAuth) Authenticate(r *http.Request) (*studio.Identity, error) {
	c, err := r.Cookie(hostCookie)
	if err != nil || c.Value == "" {
		return nil, nil
	}
	if !validName.MatchString(c.Value) {
		return nil, errors.New("invalid host session")
	}
	return &studio.Identity{
		Subject: "embed-host:" + c.Value,
		Email:   c.Value + "@embed-host.example.com",
		Name:    c.Value,
		Admin:   c.Value == "admin",
	}, nil
}

func main() {
	addr := flag.String("addr", ":8090", "listen address")
	dbPath := flag.String("db", "embed-host.db", "SQLite database for Studio")
	uiDir := flag.String("ui", "", "directory holding the unpacked UI release assets (required with -tags studio_noui)")
	chromeless := flag.Bool("chromeless", false, "render Studio's pages without its top bar and drawers, as a host that draws its own navigation would")
	proxyPort := flag.String("proxy-port", envOr("EMBED_HOST_PROXY_PORT", "9095"), "port of Studio's AI gateway (env EMBED_HOST_PROXY_PORT)")
	flag.Parse()

	// Studio's configuration comes from the host, not the environment.
	settings := map[string]string{
		"SITE_URL":            "http://localhost" + *addr + basePath,
		"BASE_PATH":           basePath,
		"DATABASE_TYPE":       "sqlite",
		"DATABASE_URL":        *dbPath,
		"TYK_AI_SECRET_KEY":   "embed-host-demo-secret-change-me",
		"CSRF_KEY":            "embed-host-demo-csrf-change-me",
		"TELEMETRY_ENABLED":   "false",
		"MARKETPLACE_ENABLED": "false",
		"DEVMODE":             "true", // plain HTTP: cookies without the Secure flag
		"PROXY_PORT":          *proxyPort,
	}
	conf := config.LoadFrom(func(key string) string { return settings[key] })

	db, err := studio.OpenDatabase(conf)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	opts := studio.Options{
		Config:    conf,
		DB:        db,
		Version:   "embed-host",
		Auth:      cookieAuth{},
		LoginURL:  "/login",
		LogoutURL: "/logout",
		// A host that draws its own navigation sets Chromeless.
		Chromeless: *chromeless,
		License:    licenseSource,
	}
	if *uiDir != "" {
		opts.UIAssets = os.DirFS(*uiDir)
	}
	s, err := studio.New(opts)
	if err != nil {
		log.Fatalf("start AI Studio: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle(basePath+"/", s.HTTPHandler())
	mux.Handle(basePath, s.HTTPHandler())
	mux.Handle("/.well-known/oauth-authorization-server"+basePath, s.OAuthMetadataHandler())
	mux.HandleFunc("/login", login)
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: hostCookie, Path: "/", MaxAge: -1})
		http.Redirect(w, r, "/login", http.StatusFound)
	})
	mux.HandleFunc("/", home)

	go func() {
		if err := s.StartProxy(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("AI gateway: %v", err)
		}
	}()

	server := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 30 * time.Second}
	go func() {
		log.Printf("embed-host listening on %s; AI Studio at %s/", *addr, basePath)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdown, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server.Shutdown(shutdown)
	if err := s.Stop(shutdown); err != nil {
		log.Printf("stop AI Studio: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.Close()
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	user := "nobody"
	if c, err := r.Cookie(hostCookie); err == nil {
		user = c.Value
	}
	fmt.Fprintf(w, `<!doctype html><title>Embed host</title>
<h1>Embed host</h1><p>Signed in as <b>%s</b>. <a href="/login">Switch user</a> · <a href="/logout">Sign out</a></p>
<p><a href="%s/">Open AI Studio</a></p>`, html.EscapeString(user), basePath)
}

func login(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		name := r.FormValue("user")
		if !validName.MatchString(name) {
			http.Error(w, "use a lower-case name", http.StatusBadRequest)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: hostCookie, Value: name, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
		http.Redirect(w, r, basePath+"/", http.StatusFound)
		return
	}
	fmt.Fprint(w, `<!doctype html><title>Embed host sign-in</title>
<h1>Embed host sign-in</h1>
<form method="post"><label>User <input name="user" value="admin"></label> <button>Sign in</button></form>
<p>"admin" is a Studio administrator; any other name is a regular user.</p>`)
}
