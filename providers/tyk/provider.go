package tyk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/TykTechnologies/midsommar/v2/providers"
)

// TykAPIDefinition represents the API definition from Tyk Dashboard
type TykAPIDefinition struct {
	APIID       string `json:"api_id"`
	Name        string `json:"name"`
	IsOAS       bool   `json:"is_oas"`
	Active      bool   `json:"active"`
	AuthType    string `json:"auth_type"`
	Protocol    string `json:"protocol"`
	ListenPath  string `json:"listen_path"`
	VersionData struct {
		Versions map[string]struct {
			Name          string `json:"name"`
			ExtendedPaths struct {
				WhiteList []struct {
					Path   string `json:"path"`
					Method string `json:"method"`
				} `json:"white_list"`
			} `json:"extended_paths"`
		} `json:"versions"`
	} `json:"version_data"`
}

// TykAPIResponse represents the response from /api/apis endpoint
type TykAPIResponse struct {
	APIs []struct {
		APIDefinition TykAPIDefinition `json:"api_definition"`
	} `json:"apis"`
}

// TykDashboardProvider implements the OpenAPIProvider interface for Tyk Dashboard
type TykDashboardProvider struct {
	Config providers.ProviderConfig
	client *http.Client
}

// NewTykDashboardProvider creates a new Tyk Dashboard provider instance
func NewTykDashboardProvider(config providers.ProviderConfig) *TykDashboardProvider {
	// Ensure URL doesn't end with a slash
	config.URL = strings.TrimSuffix(config.URL, "/")

	// If URL is a specific OAS URL, extract the base URL and API ID
	if strings.Contains(config.URL, "/api/apis/oas/") {
		parts := strings.Split(config.URL, "/api/apis/oas/")
		if len(parts) == 2 {
			config.URL = parts[0]
			config.SelectedAPIID = parts[1]
		}
	}

	return &TykDashboardProvider{
		Config: config,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the provider name
func (t *TykDashboardProvider) Name() string {
	return "Tyk Dashboard"
}

// Description returns the provider description
func (t *TykDashboardProvider) Description() string {
	return "Import API specifications from your Tyk Dashboard. Provide your Dashboard URL (e.g., http://localhost:3000) and select an API from the list."
}

// ValidateCredentials checks if the provided credentials are valid
func (t *TykDashboardProvider) ValidateCredentials() error {
	// Try to fetch the API list to validate credentials
	baseURL := strings.TrimSuffix(t.Config.URL, "/")
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/apis", baseURL), nil)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", t.Config.Token)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("invalid credentials or URL: received status code %d", resp.StatusCode)
	}

	return nil
}

// GetAPISpecs retrieves either the list of available APIs or a specific API's OAS spec
func (t *TykDashboardProvider) GetAPISpecs() ([]providers.APISpec, error) {
	// If no specific API is selected, list available APIs
	if t.Config.SelectedAPIID == "" {
		return t.listAvailableAPIs()
	}

	// If an API is selected, get its OAS spec
	return t.getAPISpec()
}

// extractOperationsAndSecurity extracts operations and security details from an OAS spec
func (t *TykDashboardProvider) extractOperationsAndSecurity(oasData map[string]interface{}) ([]string, providers.SecurityDetails) {
	var operations []string
	var securityDetails providers.SecurityDetails

	// Extract operations
	if paths, ok := oasData["paths"].(map[string]interface{}); ok {
		for path, methods := range paths {
			if methodsObj, ok := methods.(map[string]interface{}); ok {
				for method, details := range methodsObj {
					if detailsObj, ok := details.(map[string]interface{}); ok {
						// Get operationId if available
						if operationId, ok := detailsObj["operationId"].(string); ok {
							operations = append(operations, operationId)
						} else {
							// If no operationId, use method and path
							operationId = fmt.Sprintf("%s %s", strings.ToUpper(method), path)
							operations = append(operations, operationId)
						}
					}
				}
			}
		}
	}

	// Extract security details
	if security, ok := oasData["security"].([]interface{}); ok && len(security) > 0 {
		if firstScheme, ok := security[0].(map[string]interface{}); ok {
			// Get the first security scheme name
			for schemeName := range firstScheme {
				if components, ok := oasData["components"].(map[string]interface{}); ok {
					if securitySchemes, ok := components["securitySchemes"].(map[string]interface{}); ok {
						if scheme, ok := securitySchemes[schemeName].(map[string]interface{}); ok {
							if schemeType, ok := scheme["type"].(string); ok {
								securityDetails.Type = schemeType
								securityDetails.Name = schemeName // Keep the security scheme name

								// If it's an API key auth, get additional details
								if schemeType == "apiKey" {
									if in, ok := scheme["in"].(string); ok {
										securityDetails.In = in
									}
								}
							}
						}
					}
				}
			}
		}
	} else if components, ok := oasData["components"].(map[string]interface{}); ok {
		if securitySchemes, ok := components["securitySchemes"].(map[string]interface{}); ok {
			for name, scheme := range securitySchemes {
				if schemeObj, ok := scheme.(map[string]interface{}); ok {
					if schemeType, ok := schemeObj["type"].(string); ok {
						securityDetails.Type = schemeType
						securityDetails.Name = name // Keep the security scheme name

						// If it's an API key auth, get additional details
						if schemeType == "apiKey" {
							if in, ok := schemeObj["in"].(string); ok {
								securityDetails.In = in
							}
						}
						break
					}
				}
			}
		}
	}

	return operations, securityDetails
}

// listAvailableAPIs fetches the list of APIs from the dashboard
func (t *TykDashboardProvider) listAvailableAPIs() ([]providers.APISpec, error) {
	// Ensure URL doesn't end with a slash
	baseURL := strings.TrimSuffix(t.Config.URL, "/")
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/apis", baseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", t.Config.Token)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error from Tyk Dashboard: status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	var apiResp TykAPIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("error parsing API list: %w", err)
	}

	specs := make([]providers.APISpec, 0)
	for _, api := range apiResp.APIs {
		if api.APIDefinition.IsOAS && api.APIDefinition.Active {
			// Get the OAS spec for this API
			oasURL := fmt.Sprintf("%s/api/apis/oas/%s", baseURL, api.APIDefinition.APIID)
			oasReq, err := http.NewRequest("GET", oasURL, nil)
			if err != nil {
				fmt.Printf("Error creating OAS request for API %s: %v\n", api.APIDefinition.Name, err)
				continue
			}
			oasReq.Header.Set("Authorization", t.Config.Token)
			oasResp, err := t.client.Do(oasReq)
			if err != nil {
				fmt.Printf("Error fetching OAS spec for API %s: %v\n", api.APIDefinition.Name, err)
				continue
			}
			defer oasResp.Body.Close()

			var oasSpec string
			var operations []string
			var securityDetails providers.SecurityDetails

			if oasResp.StatusCode == http.StatusOK {
				oasBody, err := io.ReadAll(oasResp.Body)
				if err == nil {
					oasSpec = string(oasBody)

					// Parse OAS spec to extract operations and security details
					var oasData map[string]interface{}
					if err := json.Unmarshal(oasBody, &oasData); err == nil {
						operations, securityDetails = t.extractOperationsAndSecurity(oasData)
					}
				}
			}

			// Build a rich description including auth type and operations
			description := fmt.Sprintf("API: %s\nID: %s\nAuth Type: %s\nProtocol: %s\nListen Path: %s\n",
				api.APIDefinition.Name,
				api.APIDefinition.APIID,
				api.APIDefinition.AuthType,
				api.APIDefinition.Protocol,
				api.APIDefinition.ListenPath)

			if len(operations) > 0 {
				description += "\nOperations:\n"
				for _, op := range operations {
					description += fmt.Sprintf("- %s\n", op)
				}
			}

			specs = append(specs, providers.APISpec{
				ID:              api.APIDefinition.APIID,
				Name:            api.APIDefinition.Name,
				Description:     description,
				Source:          "tyk",
				SecurityDetails: securityDetails,
				Spec:            oasSpec,
				Operations:      operations,
			})
		}
	}

	return specs, nil
}

// getAPISpec fetches the OAS spec for a specific API
func (t *TykDashboardProvider) getAPISpec() ([]providers.APISpec, error) {
	baseURL := strings.TrimSuffix(t.Config.URL, "/")
	oasURL := fmt.Sprintf("%s/api/apis/oas/%s", baseURL, t.Config.SelectedAPIID)

	req, err := http.NewRequest("GET", oasURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", t.Config.Token)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error from Tyk Dashboard: status %d, body: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	// First get the API details to preserve the name
	apiDetails, err := t.listAvailableAPIs()
	if err != nil {
		return nil, fmt.Errorf("error getting API details: %w", err)
	}

	// Get API details from the list
	var apiName, authType, protocol, listenPath string
	for _, api := range apiDetails {
		if api.ID == t.Config.SelectedAPIID {
			apiName = api.Name
			// Get the full API definition to access all fields
			req, err := http.NewRequest("GET", fmt.Sprintf("%s/api/apis/%s", baseURL, t.Config.SelectedAPIID), nil)
			if err != nil {
				return nil, fmt.Errorf("error creating request: %w", err)
			}
			req.Header.Set("Authorization", t.Config.Token)
			resp, err := t.client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("error making request: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var apiResp struct {
					APIDefinition TykAPIDefinition `json:"api_definition"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&apiResp); err == nil {
					authType = apiResp.APIDefinition.AuthType
					protocol = apiResp.APIDefinition.Protocol
					listenPath = apiResp.APIDefinition.ListenPath
				}
			}
			break
		}
	}

	if apiName == "" {
		apiName = "Imported Tyk API Specification"
		authType = "Unknown"
		protocol = "Unknown"
		listenPath = "Unknown"
	}

	// Parse the OAS spec to extract auth and operations
	var oasSpec map[string]interface{}
	if err := json.Unmarshal(body, &oasSpec); err != nil {
		return nil, fmt.Errorf("error parsing OAS spec: %w", err)
	}

	operations, securityDetails := t.extractOperationsAndSecurity(oasSpec)

	// Build rich description
	description := fmt.Sprintf("API: %s\nID: %s\nAuth Type: %s\nProtocol: %s\nListen Path: %s\n",
		apiName, t.Config.SelectedAPIID, authType, protocol, listenPath)

	if securityDetails.Type != "" {
		description += fmt.Sprintf("\nAuthentication:\n- %s (%s)", securityDetails.Name, securityDetails.Type)
		if securityDetails.In != "" {
			description += fmt.Sprintf(" in %s", securityDetails.In)
		}
		description += "\n"
	}

	if len(operations) > 0 {
		description += "\nOperations:\n"
		for _, op := range operations {
			description += fmt.Sprintf("- %s\n", op)
		}
	}

	// Create the API spec
	specs := make([]providers.APISpec, 0, 1)
	spec := providers.APISpec{
		ID:              t.Config.SelectedAPIID,
		Name:            apiName,
		Description:     description,
		Spec:            string(body),
		Source:          "tyk",
		SecurityDetails: securityDetails,
		Operations:      operations,
	}

	specs = append(specs, spec)

	return specs, nil
}

// generateAPIKey creates a new API key for the selected API
func (t *TykDashboardProvider) generateAPIKey(baseURL string) (string, error) {
	keyPayload := map[string]interface{}{
		"allowance":          1000,
		"rate":               1000,
		"per":                1,
		"expires":            -1,
		"quota_max":          -1,
		"quota_renews":       time.Now().Unix(),
		"quota_remaining":    -1,
		"quota_renewal_rate": 60,
		"access_rights": map[string]interface{}{
			t.Config.SelectedAPIID: map[string]interface{}{
				"api_id":   t.Config.SelectedAPIID,
				"api_name": "test-api",
				"versions": []string{"Default"},
			},
		},
		"meta_data": map[string]interface{}{},
	}

	payloadBytes, err := json.Marshal(keyPayload)
	if err != nil {
		return "", fmt.Errorf("error marshaling key payload: %w", err)
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/keys", baseURL), bytes.NewBuffer(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Authorization", t.Config.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("error from Tyk Dashboard: status %d, body: %s", resp.StatusCode, string(body))
	}

	var keyResp struct {
		Key string `json:"key"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&keyResp); err != nil {
		return "", fmt.Errorf("error parsing key response: %w", err)
	}

	return keyResp.Key, nil
}
