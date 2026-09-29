package services

import (
	"strings"
	"testing"
)

// A process started outside the repository (an embedding host, say) has no
// templates directory to walk up to; the embedded defaults must render.
func TestRenderTemplateFallsBackToEmbeddedTemplates(t *testing.T) {
	t.Chdir(t.TempDir())
	s := &NotificationService{}

	out, err := s.renderTemplate("admin-notify.tmpl", map[string]interface{}{"Name": "Ada", "Email": "ada@example.com"})

	if err != nil {
		t.Fatalf("renderTemplate: %v", err)
	}
	if !strings.Contains(out, "ada@example.com") {
		t.Errorf("rendered template does not contain the data: %q", out)
	}
}
