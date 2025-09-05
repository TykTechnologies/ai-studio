// internal/services/model_converters.go
package services

import (
	"encoding/json"
	"fmt"

	"github.com/TykTechnologies/midsommar/microgateway/internal/database"
)

// Model conversion functions for compatibility with midsommar models
// These functions convert between internal database models and external service models

// TempMidsommarModels - temporary models until full integration
type MidsommarLLM struct {
	ID            uint                   `json:"id"`
	Name          string                 `json:"name"`
	Slug          string                 `json:"slug"`
	Vendor        string                 `json:"vendor"`
	Endpoint      string                 `json:"endpoint"`
	DefaultModel  string                 `json:"default_model"`
	MaxTokens     int                    `json:"max_tokens"`
	Timeout       int                    `json:"timeout_seconds"`
	RetryCount    int                    `json:"retry_count"`
	IsActive      bool                   `json:"is_active"`
	MonthlyBudget float64                `json:"monthly_budget"`
	RateLimit     int                    `json:"rate_limit_rpm"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

type MidsommarCredential struct {
	ID         uint   `json:"id"`
	AppID      uint   `json:"app_id"`
	KeyID      string `json:"key_id"`
	Name       string `json:"name,omitempty"`
	IsActive   bool   `json:"is_active"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at,omitempty"`
	LastUsedAt string `json:"last_used_at,omitempty"`
}

type MidsommarApp struct {
	ID            uint                   `json:"id"`
	Name          string                 `json:"name"`
	Description   string                 `json:"description,omitempty"`
	OwnerEmail    string                 `json:"owner_email"`
	IsActive      bool                   `json:"is_active"`
	MonthlyBudget float64                `json:"monthly_budget"`
	RateLimit     int                    `json:"rate_limit_rpm"`
	AllowedIPs    []string               `json:"allowed_ips,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	LLMs          []MidsommarLLM         `json:"llms,omitempty"`
	Credentials   []MidsommarCredential  `json:"credentials,omitempty"`
}

// ConvertToMidsommarLLM converts database.LLM to midsommar-compatible model
func ConvertToMidsommarLLM(dbLLM database.LLM) MidsommarLLM {
	var metadata map[string]interface{}
	if len(dbLLM.Metadata) > 0 {
		json.Unmarshal(dbLLM.Metadata, &metadata)
	}

	return MidsommarLLM{
		ID:            dbLLM.ID,
		Name:          dbLLM.Name,
		Slug:          dbLLM.Slug,
		Vendor:        dbLLM.Vendor,
		Endpoint:      dbLLM.Endpoint,
		DefaultModel:  dbLLM.DefaultModel,
		MaxTokens:     dbLLM.MaxTokens,
		Timeout:       dbLLM.TimeoutSeconds,
		RetryCount:    dbLLM.RetryCount,
		IsActive:      dbLLM.IsActive,
		MonthlyBudget: dbLLM.MonthlyBudget,
		RateLimit:     dbLLM.RateLimitRPM,
		Metadata:      metadata,
	}
}

// ConvertToMidsommarCredential converts database.Credential to midsommar-compatible model
func ConvertToMidsommarCredential(dbCred database.Credential) MidsommarCredential {
	cred := MidsommarCredential{
		ID:        dbCred.ID,
		AppID:     dbCred.AppID,
		KeyID:     dbCred.KeyID,
		Name:      dbCred.Name,
		IsActive:  dbCred.IsActive,
		CreatedAt: dbCred.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if dbCred.ExpiresAt != nil {
		cred.ExpiresAt = dbCred.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if dbCred.LastUsedAt != nil {
		cred.LastUsedAt = dbCred.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return cred
}

// ConvertToMidsommarApp converts database.App to midsommar-compatible model
func ConvertToMidsommarApp(dbApp database.App) MidsommarApp {
	var allowedIPs []string
	if len(dbApp.AllowedIPs) > 0 {
		json.Unmarshal(dbApp.AllowedIPs, &allowedIPs)
	}

	var metadata map[string]interface{}
	if len(dbApp.Metadata) > 0 {
		json.Unmarshal(dbApp.Metadata, &metadata)
	}

	app := MidsommarApp{
		ID:            dbApp.ID,
		Name:          dbApp.Name,
		Description:   dbApp.Description,
		OwnerEmail:    dbApp.OwnerEmail,
		IsActive:      dbApp.IsActive,
		MonthlyBudget: dbApp.MonthlyBudget,
		RateLimit:     dbApp.RateLimitRPM,
		AllowedIPs:    allowedIPs,
		Metadata:      metadata,
	}

	// Convert associated LLMs
	if len(dbApp.LLMs) > 0 {
		app.LLMs = make([]MidsommarLLM, len(dbApp.LLMs))
		for i, llm := range dbApp.LLMs {
			app.LLMs[i] = ConvertToMidsommarLLM(llm)
		}
	}

	// Convert credentials
	if len(dbApp.Credentials) > 0 {
		app.Credentials = make([]MidsommarCredential, len(dbApp.Credentials))
		for i, cred := range dbApp.Credentials {
			app.Credentials[i] = ConvertToMidsommarCredential(cred)
		}
	}

	return app
}

// ConvertFromMidsommarLLM converts midsommar LLM model to database model
func ConvertFromMidsommarLLM(llm MidsommarLLM) database.LLM {
	var metadata []byte
	if llm.Metadata != nil {
		metadata, _ = json.Marshal(llm.Metadata)
	}

	return database.LLM{
		Name:           llm.Name,
		Slug:           llm.Slug,
		Vendor:         llm.Vendor,
		Endpoint:       llm.Endpoint,
		DefaultModel:   llm.DefaultModel,
		MaxTokens:      llm.MaxTokens,
		TimeoutSeconds: llm.Timeout,
		RetryCount:     llm.RetryCount,
		IsActive:       llm.IsActive,
		MonthlyBudget:  llm.MonthlyBudget,
		RateLimitRPM:   llm.RateLimit,
		Metadata:       metadata,
	}
}

// ConvertLLMList converts a list of database LLMs to midsommar models
func ConvertLLMList(dbLLMs []database.LLM) []MidsommarLLM {
	result := make([]MidsommarLLM, len(dbLLMs))
	for i, llm := range dbLLMs {
		result[i] = ConvertToMidsommarLLM(llm)
	}
	return result
}

// ConvertAppList converts a list of database Apps to midsommar models
func ConvertAppList(dbApps []database.App) []MidsommarApp {
	result := make([]MidsommarApp, len(dbApps))
	for i, app := range dbApps {
		result[i] = ConvertToMidsommarApp(app)
	}
	return result
}

// ValidateCreateLLMRequest validates and normalizes create LLM request
func ValidateCreateLLMRequest(req *CreateLLMRequest) error {
	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 4096
	}
	if req.TimeoutSeconds == 0 {
		req.TimeoutSeconds = 30
	}
	if req.RetryCount == 0 {
		req.RetryCount = 3
	}

	// Validate required fields
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Vendor == "" {
		return fmt.Errorf("vendor is required")
	}
	if req.DefaultModel == "" {
		return fmt.Errorf("default_model is required")
	}

	// Vendor-specific validation
	validVendors := map[string]bool{
		"openai":    true,
		"anthropic": true,
		"google":    true,
		"vertex":    true,
		"ollama":    true,
	}

	if !validVendors[req.Vendor] {
		return fmt.Errorf("unsupported vendor: %s", req.Vendor)
	}

	// Vendor-specific requirements
	switch req.Vendor {
	case "openai", "anthropic":
		if req.APIKey == "" {
			return fmt.Errorf("API key is required for %s", req.Vendor)
		}
	case "ollama":
		if req.Endpoint == "" {
			return fmt.Errorf("endpoint is required for Ollama")
		}
	}

	return nil
}