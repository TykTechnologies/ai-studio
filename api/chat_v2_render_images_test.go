package api

import (
	"testing"

	"github.com/TykTechnologies/midsommar/v2/models"
)

// The v2 chat UI reads the chat's image setting from the session summary
// (2u2).
func TestChatSummaryCarriesRenderImages(t *testing.T) {
	if chatSummary(&models.Chat{}).RenderImages {
		t.Error("render_images is on for a chat that did not set it")
	}
	if !chatSummary(&models.Chat{RenderImages: true}).RenderImages {
		t.Error("render_images is lost from the session summary")
	}
}
