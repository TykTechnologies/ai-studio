package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/TykTechnologies/midsommar/v2/logger"
	"github.com/TykTechnologies/midsommar/v2/models"
)

// ErrMCPServerNotVisible is returned when an App would be bound to an MCP
// server the caller's teams cannot see, or one that is not published and
// active on its Dashboard.
var ErrMCPServerNotVisible = errors.New("MCP server is not available to this app")

// ErrMCPServerNotBrokerable is returned when an App would be bound to an MCP
// server AI Studio issues no key for (OAuth, mTLS, keyless, or no policy
// bundle pinned yet): the App would grant nothing, so it is refused.
var ErrMCPServerNotBrokerable = errors.New("AI Studio cannot issue a key for this MCP server; connect to it directly")

// ValidateMCPServerBindings checks that every server may be bound by this
// user to an App with the given providers: visible (team grant, or any
// published server for an administrator), published and active on the
// Dashboard, and within the privacy rule (no server above the App's
// highest-privacy provider). It returns the servers in the order given.
func (s *Service) ValidateMCPServerBindings(userID uint, isAdmin bool, llmIDs []uint, serverIDs []uint) ([]models.MCPServer, error) {
	ids := uniqueUintIDs(serverIDs)
	if len(ids) == 0 {
		return nil, nil
	}
	var q = s.DB.Model(&models.MCPServer{})
	if isAdmin {
		q = q.Where("mcp_servers.is_active = ? AND mcp_servers.dashboard_state = ?", true, models.MCPDashboardActive)
	} else {
		q = models.AccessibleMCPServerQuery(s.DB, userID)
	}
	var servers []models.MCPServer
	if err := q.Where("mcp_servers.id IN ?", ids).Group("mcp_servers.id").Find(&servers).Error; err != nil {
		return nil, err
	}
	byID := map[uint]models.MCPServer{}
	for _, srv := range servers {
		byID[srv.ID] = srv
	}
	ordered := make([]models.MCPServer, 0, len(ids))
	for _, id := range ids {
		srv, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("%w: server %d", ErrMCPServerNotVisible, id)
		}
		if !srv.Brokerable {
			return nil, fmt.Errorf("%w: %s", ErrMCPServerNotBrokerable, srv.Name)
		}
		ordered = append(ordered, srv)
	}

	// Like tools, MCP servers are only compared once the App has a provider:
	// an App that exists to hold a credential for an external MCP client has
	// nothing to leak to.
	if len(llmIDs) == 0 {
		return ordered, nil
	}
	llms, err := s.loadLLMPrivacyScores(llmIDs)
	if err != nil {
		return nil, err
	}
	maxLLMScore, maxLLMName := -1, ""
	for _, llmID := range llmIDs {
		if llm := llms[llmID]; llm.Score > maxLLMScore {
			maxLLMScore, maxLLMName = llm.Score, llm.Name
		}
	}
	for _, srv := range ordered {
		if srv.PrivacyScore == nil {
			continue
		}
		if *srv.PrivacyScore > maxLLMScore {
			return nil, &PrivacyScoreMismatch{
				ResourceKind: "MCP server", ResourceName: srv.Name, ResourceScore: *srv.PrivacyScore,
				LLMName: maxLLMName, MaxLLMScore: maxLLMScore,
			}
		}
	}
	return ordered, nil
}

// SetAppMCPServers replaces the App's MCP server bindings and reconciles the
// access-grant ledger. Callers validate with ValidateMCPServerBindings first.
func (s *Service) SetAppMCPServers(appID uint, serverIDs []uint) error {
	var app models.App
	if err := s.DB.First(&app, appID).Error; err != nil {
		return err
	}
	ids := uniqueUintIDs(serverIDs)
	var servers []models.MCPServer
	if len(ids) > 0 {
		if err := s.DB.Where("id IN ?", ids).Find(&servers).Error; err != nil {
			return err
		}
	}
	if err := s.DB.Model(&app).Association("MCPServers").Replace(servers); err != nil {
		return fmt.Errorf("failed to set app MCP servers: %w", err)
	}
	s.syncAppMCPGrants(appID)
	return nil
}

// ClearAppMCPServers removes every MCP binding (App deletion).
func (s *Service) ClearAppMCPServers(appID uint) error {
	app := models.App{}
	app.ID = appID
	if err := s.DB.Model(&app).Association("MCPServers").Clear(); err != nil {
		return err
	}
	return nil
}

// GetAppMCPServers lists the MCP servers bound to an App.
func (s *Service) GetAppMCPServers(appID uint) ([]models.MCPServer, error) {
	var app models.App
	if err := s.DB.Preload("MCPServers").Preload("MCPServers.Connection").First(&app, appID).Error; err != nil {
		return nil, err
	}
	return app.MCPServers, nil
}

// syncAppMCPGrants asks the Tyk MCP integration (Enterprise) to reconcile
// the grant ledger. Best effort: a failure is logged, never surfaced to the
// App operation that triggered it.
func (s *Service) syncAppMCPGrants(appID uint) {
	if s.TykMCP == nil {
		return
	}
	if err := s.TykMCP.SyncAppGrants(context.Background(), appID); err != nil {
		logAppMCPGrantSyncFailure(appID, err)
	}
}

func logAppMCPGrantSyncFailure(appID uint, err error) {
	logger.Warnf("mcp access grants for app %d not reconciled: %v", appID, err)
}

func uniqueUintIDs(in []uint) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0, len(in))
	for _, v := range in {
		if v == 0 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
