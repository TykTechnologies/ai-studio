package docs

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"time"
)

//go:embed site/public
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
	publicFS, err := fs.Sub(content, "site/public")
	if err != nil {
		return fmt.Errorf("failed to get public subfolder: %w", err)
	}

	// Create file server from embedded files
	fs := http.FileServer(http.FS(publicFS))

	// Add basic logging middleware
	handler := logMiddleware(fs)

	s.Router.Handle("/", handler)

	addr := fmt.Sprintf(":%d", s.Port)
	fmt.Printf("Starting documentation server at :%s\n", addr)

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
