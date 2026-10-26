package services

import (
	"strings"
	"testing"
)

// A process started outside the repository (an embedding host, say) has no
// templates directory to walk up to; the embedded defaults must render.
// The working directory is / rather than a fresh temporary directory, whose
// name would change go test's result cache key on every run.
func TestRenderTemplateFallsBackToEmbeddedTemplates(t *testing.T) {
	t.Chdir("/")
	s := &NotificationService{}

	out, err := s.renderTemplate("admin-notify.tmpl", map[string]interface{}{"Name": "Ada", "Email": "ada@example.com"})

	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(out, "ada@example.com") {
		t.Errorf("rendered template does not contain the data: %q", out)
	}
}
