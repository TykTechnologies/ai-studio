package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/logger"
)

//go:embed site/dist
var content embed.FS

// Server represents the local web server
type Server struct {
	Port   int
	Router *http.ServeMux
}

// NewServer creates a new server instance with the specified port
func NewServer(port int) *Server {
	return &Server{
		Port:   port,
		Router: http.NewServeMux(),
	}
}

// Start initializes and starts the web server
func (s *Server) Start() error {
	// Get the public directory as a sub-filesystem
	publicFS, err := fs.Sub(content, "site/dist")
	if err != nil {
		return fmt.Errorf("failed to get public subfolder: %w", err)
	}

	// Create file server from embedded files
	fileServer := http.FileServer(http.FS(publicFS))

	// Wrap with handler that strips /ai-studio/ prefix for compatibility
	// with VitePress production builds that use base: '/ai-studio/'
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip /ai-studio prefix if present
		if strings.HasPrefix(r.URL.Path, "/ai-studio/") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/ai-studio")
		} else if r.URL.Path == "/ai-studio" {
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})

	s.Router.Handle("/", logMiddleware(handler))

	addr := fmt.Sprintf(":%d", s.Port)
	logger.Infof("Starting documentation server at %s", addr)

	server := &http.Server{
		Addr:         addr,
		Handler:      s.Router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return server.ListenAndServe()
}

// logMiddleware adds basic request logging
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// start := time.Now()
		next.ServeHTTP(w, r)
		// fmt.Printf("%s %s %s\n", r.Method, r.URL.Path, time.Since(start))
	})
}
