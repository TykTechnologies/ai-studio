package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/TykTechnologies/midsommar/v2/pkg/plugin_sdk"
	pb "github.com/TykTechnologies/midsommar/v2/proto"
)

//go:embed manifest.json
var manifestFile []byte

//go:embed ui/*
var uiAssets embed.FS

// FeedbackEntry represents a single feedback submission
type FeedbackEntry struct {
	ID        string `json:"id"`
	UserID    uint32 `json:"user_id"`
	UserEmail string `json:"user_email"`
	UserName  string `json:"user_name"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	Rating    int    `json:"rating"`
	CreatedAt string `json:"created_at"`
}

// PortalFeedbackPlugin demonstrates both admin and portal UI capabilities
type PortalFeedbackPlugin struct {
	plugin_sdk.BasePlugin
	mu       sync.RWMutex
	feedback []FeedbackEntry
	nextID   int
}

func NewPortalFeedbackPlugin() *PortalFeedbackPlugin {
	return &PortalFeedbackPlugin{
		BasePlugin: plugin_sdk.NewBasePlugin(
			"portal-feedback",
			"1.0.0",
			"Example plugin demonstrating portal UI with user feedback form",
		),
		feedback: make([]FeedbackEntry, 0),
		nextID:   1,
	}
}

func (p *PortalFeedbackPlugin) Initialize(ctx plugin_sdk.Context, config map[string]string) error {
	ctx.Services.Logger().Info("Portal feedback plugin initialized")
	return nil
}

// === UIProvider implementation (for admin UI + asset serving) ===

func (p *PortalFeedbackPlugin) GetAsset(assetPath string) ([]byte, string, error) {
	// Strip leading slash
	path := strings.TrimPrefix(assetPath, "/")

	// Try to read from embedded UI assets
	content, err := uiAssets.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("asset not found: %s", path)
	}

	mimeType := "application/octet-stream"
	if strings.HasSuffix(path, ".js") {
		mimeType = "application/javascript"
	} else if strings.HasSuffix(path, ".css") {
		mimeType = "text/css"
	} else if strings.HasSuffix(path, ".html") {
		mimeType = "text/html"
	}

	return content, mimeType, nil
}

func (p *PortalFeedbackPlugin) ListAssets(pathPrefix string) ([]*pb.AssetInfo, error) {
	return nil, nil
}

func (p *PortalFeedbackPlugin) GetManifest() ([]byte, error) {
	return manifestFile, nil
}

// HandleRPC processes admin RPC calls (admin-only)
func (p *PortalFeedbackPlugin) HandleRPC(method string, payload []byte) ([]byte, error) {
	switch method {
	case "list_feedback":
		return p.rpcListFeedback()
	case "delete_feedback":
		return p.rpcDeleteFeedback(payload)
	default:
		return nil, fmt.Errorf("unknown admin RPC method: %s", method)
	}
}

// === PortalUIProvider implementation (for portal UI) ===

// HandlePortalRPC processes portal RPC calls (any authenticated user)
func (p *PortalFeedbackPlugin) HandlePortalRPC(method string, payload []byte, userCtx *plugin_sdk.PortalUserContext) ([]byte, error) {
	switch method {
	case "submit_feedback":
		return p.rpcSubmitFeedback(payload, userCtx)
	case "my_feedback":
		return p.rpcMyFeedback(userCtx)
	default:
		return nil, fmt.Errorf("unknown portal RPC method: %s", method)
	}
}

// === RPC Implementations ===

func (p *PortalFeedbackPlugin) rpcSubmitFeedback(payload []byte, userCtx *plugin_sdk.PortalUserContext) ([]byte, error) {
	var req struct {
		Title   string `json:"title"`
		Message string `json:"message"`
		Rating  int    `json:"rating"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.Title == "" || req.Message == "" {
		return json.Marshal(map[string]interface{}{
			"success": false,
			"error":   "Title and message are required",
		})
	}

	if req.Rating < 1 || req.Rating > 5 {
		return json.Marshal(map[string]interface{}{
			"success": false,
			"error":   "Rating must be between 1 and 5",
		})
	}

	p.mu.Lock()
	entry := FeedbackEntry{
		ID:        fmt.Sprintf("fb_%d", p.nextID),
		UserID:    userCtx.UserID,
		UserEmail: userCtx.Email,
		UserName:  userCtx.Name,
		Title:     req.Title,
		Message:   req.Message,
		Rating:    req.Rating,
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	p.nextID++
	p.feedback = append(p.feedback, entry)
	p.mu.Unlock()

	return json.Marshal(map[string]interface{}{
		"success":  true,
		"feedback": entry,
	})
}

func (p *PortalFeedbackPlugin) rpcMyFeedback(userCtx *plugin_sdk.PortalUserContext) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var myFeedback []FeedbackEntry
	for _, fb := range p.feedback {
		if fb.UserID == userCtx.UserID {
			myFeedback = append(myFeedback, fb)
		}
	}

	return json.Marshal(map[string]interface{}{
		"feedback": myFeedback,
	})
}

func (p *PortalFeedbackPlugin) rpcListFeedback() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return json.Marshal(map[string]interface{}{
		"feedback": p.feedback,
		"total":    len(p.feedback),
	})
}

func (p *PortalFeedbackPlugin) rpcDeleteFeedback(payload []byte) ([]byte, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for i, fb := range p.feedback {
		if fb.ID == req.ID {
			p.feedback = append(p.feedback[:i], p.feedback[i+1:]...)
			return json.Marshal(map[string]interface{}{"success": true})
		}
	}

	return json.Marshal(map[string]interface{}{
		"success": false,
		"error":   "Feedback not found",
	})
}

func main() {
	plugin_sdk.Serve(NewPortalFeedbackPlugin())
}
