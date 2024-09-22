package universalclient

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pb33f/libopenapi"
	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/tmc/langchaingo/llms"
	"gopkg.in/yaml.v3"
)

type ResponseFormat int

const (
	ResponseFormatDefault ResponseFormat = iota
	ResponseFormatJSON
	ResponseFormatXML
)

type AuthMethod int

const (
	AuthNone AuthMethod = iota
	AuthBearer
	AuthBasic
	AuthApiKey
	AuthOAuth2
	// Add more auth methods as needed
)

type AuthSchemeInfo struct {
	Name        string
	Type        string
	Description string
	In          string // For APIKey auth, specifies whether it's in header or query
	KeyName     string // For APIKey auth, specifies the name of the key
}

type AuthConfig struct {
	Schemes []AuthScheme
}

type AuthScheme struct {
	Method     AuthMethod
	Name       string // The name of the scheme in the OpenAPI spec
	Token      string
	Username   string
	Password   string
	ApiKeyName string
	ApiKeyIn   string // 'header' or 'query'
}

// OperationInputs represents the expected inputs for an operation
type OperationInputs struct {
	PathParams  []ParameterInfo
	QueryParams []ParameterInfo
	RequestBody *RequestBodyInfo
}

// ParameterInfo represents information about a parameter
type ParameterInfo struct {
	Name        string
	Description string
	Required    bool
	Schema      *base.Schema
}

// RequestBodyInfo represents information about the request body
type RequestBodyInfo struct {
	Description string
	Required    bool
	ContentType string
	Schema      *base.Schema
}

// Client represents an API client generated from an OpenAPI specification
type Client struct {
	spec           libopenapi.Document
	resolvedNode   *yaml.Node
	baseURL        string
	httpClient     *http.Client
	responseFormat ResponseFormat
	authConfig     AuthConfig
}

// ClientOption is a function that configures a Client
type ClientOption func(*Client)

// NewClient creates a new API client from an OpenAPI specification
func NewClient(specBytes []byte, baseURL string, options ...ClientOption) (*Client, error) {
	// Parse the OpenAPI specification
	doc, err := libopenapi.NewDocument(specBytes)
	if err != nil {
		return nil, err
	}

	// Validate the specification
	if err := validateSpec(doc); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI specification: %w", err)
	}

	// Create the client with default settings
	client := &Client{
		spec: doc,
		httpClient: &http.Client{
			Timeout: time.Second * 30,
		},
		responseFormat: ResponseFormatDefault,
	}

	if err := client.parseSecuritySchemes(); err != nil {
		return nil, err
	}

	// If baseURL is not provided, try to get it from the spec
	if baseURL == "" {
		baseURL, err = getBaseURLFromSpec(doc)
		if err != nil {
			return nil, err
		}
	}

	client.baseURL = baseURL

	// Apply any custom options
	for _, option := range options {
		option(client)
	}

	return client, nil
}

func validateSpec(doc libopenapi.Document) error {
	model, err := doc.BuildV3Model()
	if err != nil {
		return fmt.Errorf("failed to build V3 model: %v", err)
	}

	// Check OpenAPI version
	if model.Model.Version != "3.0.0" && !strings.HasPrefix(model.Model.Version, "3.") {
		return fmt.Errorf("unsupported OpenAPI version: %s. Only version 3.x is supported", model.Model.Version)
	}

	// Check for valid servers entry
	if len(model.Model.Servers) == 0 {
		return fmt.Errorf("specification must have at least one valid servers entry")
	}

	// Check for SecuritySchemes
	if model.Model.Components == nil || model.Model.Components.SecuritySchemes == nil || model.Model.Components.SecuritySchemes.Len() == 0 {
		return fmt.Errorf("specification must have at least one SecuritySchema entry")
	}

	// Validate SecuritySchemes
	hasValidAuthScheme := false
	for pair := model.Model.Components.SecuritySchemes.First(); pair != nil; pair = pair.Next() {
		scheme := pair.Value()
		switch scheme.Type {
		case "apiKey":
			hasValidAuthScheme = true
		case "http":
			if scheme.Scheme == "bearer" || scheme.Scheme == "basic" {
				hasValidAuthScheme = true
			}
		}
	}
	if !hasValidAuthScheme {
		return fmt.Errorf("specification must have at least one supported authentication type (apiKey, bearer, or basic)")
	}

	// Check all paths for operationID
	if model.Model.Paths != nil && model.Model.Paths.PathItems != nil {
		for pair := model.Model.Paths.PathItems.First(); pair != nil; pair = pair.Next() {
			pathItem := pair.Value()
			if pathItem == nil {
				continue
			}
			operations := []*v3.Operation{
				pathItem.Get, pathItem.Post, pathItem.Put, pathItem.Delete,
				pathItem.Options, pathItem.Head, pathItem.Patch, pathItem.Trace,
			}
			for _, op := range operations {
				if op != nil && op.OperationId == "" {
					return fmt.Errorf("all operations must have an operationID")
				}
			}
		}
	}

	return nil
}

func getBaseURLFromSpec(doc libopenapi.Document) (string, error) {
	model, err := doc.BuildV3Model()
	if err != nil {
		return "", fmt.Errorf("failed to build V3 model: %v", err)
	}

	if len(model.Model.Servers) > 0 {
		// Use the first server URL
		return model.Model.Servers[0].URL, nil
	}

	return "", fmt.Errorf("no server URL found in the OpenAPI specification")
}

func WithResponseFormat(format ResponseFormat) ClientOption {
	return func(c *Client) {
		c.responseFormat = format
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

// WithTimeout sets a custom timeout for the HTTP client
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

func WithAuth(schemeName string, credentials interface{}) ClientOption {
	return func(c *Client) {
		setAuth := false
		for i, scheme := range c.authConfig.Schemes {
			if scheme.Name == schemeName {
				switch creds := credentials.(type) {
				case string:
					switch scheme.Method {
					case AuthBearer, AuthApiKey:
						c.authConfig.Schemes[i].Token = creds
					case AuthBasic:
						parts := strings.SplitN(creds, ":", 2)
						if len(parts) == 2 {
							c.authConfig.Schemes[i].Username = parts[0]
							c.authConfig.Schemes[i].Password = parts[1]
						}
					}
					// Add more cases if needed for different credential types
				}
				setAuth = true
				break
			}
		}

		if !setAuth {
			// search the other way around, by type
			for i, scheme := range c.authConfig.Schemes {
				if scheme.Method == AuthApiKey || scheme.Method == AuthBearer {
					c.authConfig.Schemes[i].Token = credentials.(string)
					break
				}
			}
		}
	}
}

func (c *Client) GetAuthSchemes() []AuthScheme {
	return c.authConfig.Schemes
}

func (c *Client) GetSpec() libopenapi.Document {
	return c.spec
}

func (c *Client) GetInputSpecForOperation(operationId string) (map[string]interface{}, error) {
	operation, _, _, err := c.findOperation(operationId)
	if err != nil {
		return nil, err
	}

	schemas := c.buildParametersSchema(operation)
	return schemas, nil
}

func (c *Client) CallOperation(operationId string, params map[string][]string, payload map[string]interface{}, headers map[string][]string) (interface{}, error) {
	// Find the operation in the OpenAPI document
	operation, path, method, err := c.findOperation(operationId)
	if err != nil {
		return nil, err
	}

	// Construct the URL
	url, err := c.constructURL(path, params)
	if err != nil {
		return nil, err
	}

	// Prepare the request body
	var body io.Reader
	if payload != nil {
		jsonBody, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
		body = bytes.NewBuffer(jsonBody)
	}

	// Create the request
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for k, v := range headers {
		req.Header[k] = v
	}

	// Set content type if not provided
	if _, ok := req.Header["Content-Type"]; !ok && payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Apply authentication
	for _, scheme := range c.authConfig.Schemes {
		switch scheme.Method {
		case AuthBearer:
			req.Header.Set("Authorization", "Bearer "+scheme.Token)
		case AuthBasic:
			req.SetBasicAuth(scheme.Username, scheme.Password)
		case AuthApiKey:
			if scheme.ApiKeyIn == "header" {
				req.Header.Set(scheme.ApiKeyName, scheme.Token)
			} else if scheme.ApiKeyIn == "query" {
				q := req.URL.Query()
				q.Add(scheme.ApiKeyName, scheme.Token)
				req.URL.RawQuery = q.Encode()
			}
			// Add more cases as needed
		}
	}

	// Send the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for successful status code
	if resp.StatusCode < 200 || resp.StatusCode >= 401 {
		// somethig wrong with auth
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("request failed with status code: %v error: %v", string(body), resp.StatusCode)
	}

	// Parse the response
	return c.parseResponse(resp, operation)
}

func (c *Client) findOperation(operationId string) (*v3.Operation, string, string, error) {
	model, err := c.spec.BuildV3Model()
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to build V3 model: %v", err)
	}

	pathItems := model.Model.Paths.PathItems

	for pair := pathItems.First(); pair != nil; pair = pair.Next() {
		path := pair.Key()
		pathItem := pair.Value()

		methods := map[string]*v3.Operation{
			"GET":     pathItem.Get,
			"POST":    pathItem.Post,
			"PUT":     pathItem.Put,
			"DELETE":  pathItem.Delete,
			"OPTIONS": pathItem.Options,
			"HEAD":    pathItem.Head,
			"PATCH":   pathItem.Patch,
			"TRACE":   pathItem.Trace,
		}

		for method, op := range methods {
			if op != nil && op.OperationId == operationId {
				return op, path, method, nil
			}
		}
	}

	return nil, "", "", fmt.Errorf("operation not found: %s", operationId)
}

func (c *Client) constructURL(path string, params map[string][]string) (string, error) {
	// Replace path parameters
	for k, v := range params {
		if strings.Contains(path, fmt.Sprintf("{%s}", k)) {
			path = strings.ReplaceAll(path, fmt.Sprintf("{%s}", k), url.PathEscape(v[0]))
			delete(params, k)
		}
	}

	// Add query parameters
	u, err := url.Parse(c.baseURL + path)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}
	q := u.Query()
	for k, v := range params {
		for _, vv := range v {
			q.Add(k, vv)
		}
	}
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func (c *Client) parseResponse(resp *http.Response, operation *v3.Operation) (interface{}, error) {
	contentType := resp.Header.Get("Content-Type")
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	responseSchema := c.getResponseSchema(operation, resp.StatusCode)
	if responseSchema == nil {
		// default to text response
		c.responseFormat = ResponseFormatDefault
		contentType = "text/text"
	}

	switch c.responseFormat {
	case ResponseFormatJSON:
		return c.parseJSONResponse(responseBody, responseSchema)
	case ResponseFormatXML:
		return c.parseXMLResponse(responseBody, responseSchema)
	default:
		switch {
		case strings.HasPrefix(contentType, "application/json"):
			return c.parseJSONResponse(responseBody, responseSchema)
		case strings.HasPrefix(contentType, "application/xml"):
			return c.parseXMLResponse(responseBody, responseSchema)
		case strings.HasPrefix(contentType, "text/"):
			return string(responseBody), nil
		default:
			return responseBody, nil
		}
	}
}

func (c *Client) getResponseSchema(operation *v3.Operation, statusCode int) *base.SchemaProxy {
	if operation.Responses == nil {
		return nil
	}

	statusCodeStr := fmt.Sprintf("%d", statusCode)
	var response *v3.Response

	// Check for the specific status code
	if operation.Responses.Codes != nil {
		var ok bool
		response, ok = operation.Responses.Codes.Get(statusCodeStr)
		if !ok {
			response, _ = operation.Responses.Codes.Get("default")
		}
	}

	// If not found, check for default response
	if response == nil {
		response = operation.Responses.Default
	}

	if response == nil || response.Content == nil {
		return nil
	}

	// Prefer JSON schema, fallback to any available schema
	for _, mediaType := range []string{"application/json", "*/*"} {
		if content, ok := response.Content.Get(mediaType); ok && content.Schema != nil {
			return content.Schema
		}
	}

	return nil
}

func (c *Client) parseJSONResponse(data []byte, schema *base.SchemaProxy) (interface{}, error) {
	if c.responseFormat == ResponseFormatJSON {
		return string(data), nil
	}

	var result interface{}

	// Get the actual schema from the proxy
	actualSchema := schema.Schema()

	if actualSchema == nil || len(actualSchema.Type) == 0 {
		// If we can't get the actual schema or it has no type, default to map[string]interface{}
		result = make(map[string]interface{})
	} else {
		// Use the first type in the slice as the primary type
		primaryType := actualSchema.Type[0]
		switch primaryType {
		case "object":
			result = make(map[string]interface{})
		case "array":
			result = make([]interface{}, 0)
		case "string":
			result = ""
		case "number", "integer":
			result = 0.0
		case "boolean":
			result = false
		default:
			// For unknown types, fall back to map[string]interface{}
			result = make(map[string]interface{})
		}
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Additional type checking and conversion could be done here if needed
	// For example, handling "null" type or multiple types

	return result, nil
}

func (c *Client) parseXMLResponse(data []byte, schema *base.SchemaProxy) (interface{}, error) {
	if c.responseFormat == ResponseFormatXML {
		return string(data), nil
	}

	// XML parsing is more complex and may require a custom implementation
	// For now, we'll use a simple map[string]interface{} approach
	var result interface{}

	// Get the actual schema from the proxy
	actualSchema := schema.Schema()

	if actualSchema != nil && len(actualSchema.Type) > 0 {
		// Use the first type in the slice as the primary type
		primaryType := actualSchema.Type[0]
		switch primaryType {
		case "object":
			result = make(map[string]interface{})
		case "array":
			result = make([]interface{}, 0)
		default:
			// For other types, we'll still use map[string]interface{} as XML usually represents structured data
			result = make(map[string]interface{})
		}
	} else {
		// Default to map[string]interface{} if no schema or type is provided
		result = make(map[string]interface{})
	}

	if err := xml.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse XML response: %w", err)
	}

	return result, nil
}

func (c *Client) ListOperations() ([]string, error) {
	model, err := c.spec.BuildV3Model()
	if err != nil {
		return nil, fmt.Errorf("failed to build V3 model: %v", err)
	}

	operations := []string{}
	pathItems := model.Model.Paths.PathItems

	for pair := pathItems.First(); pair != nil; pair = pair.Next() {
		pathItem := pair.Value()

		methods := []*v3.Operation{
			pathItem.Get,
			pathItem.Post,
			pathItem.Put,
			pathItem.Delete,
			pathItem.Options,
			pathItem.Head,
			pathItem.Patch,
			pathItem.Trace,
		}

		for _, op := range methods {
			if op != nil && op.OperationId != "" {
				operations = append(operations, op.OperationId)
			}
		}
	}

	return operations, nil
}

// GetOperationInputs returns the expected inputs for a given operation ID
func (c *Client) GetOperationInputs(operationId string) (*OperationInputs, error) {
	operation, _, _, err := c.findOperation(operationId)
	if err != nil {
		return nil, err
	}

	inputs := &OperationInputs{
		PathParams:  []ParameterInfo{},
		QueryParams: []ParameterInfo{},
	}

	// Process parameters
	for _, param := range operation.Parameters {
		info := ParameterInfo{
			Name:        param.Name,
			Description: param.Description,
			Required:    *param.Required,
			Schema:      param.Schema.Schema(),
		}

		switch param.In {
		case "path":
			inputs.PathParams = append(inputs.PathParams, info)
		case "query":
			inputs.QueryParams = append(inputs.QueryParams, info)
		}
	}

	// Process request body
	if operation.RequestBody != nil {
		inputs.RequestBody = &RequestBodyInfo{
			Description: operation.RequestBody.Description,
			Required:    *operation.RequestBody.Required,
		}

		// Assume JSON content type for simplicity
		if content, ok := operation.RequestBody.Content.Get("application/json"); ok {
			inputs.RequestBody.ContentType = "application/json"
			inputs.RequestBody.Schema = content.Schema.Schema()
		}

	}

	return inputs, nil
}

func (c *Client) parseSecuritySchemes() error {
	model, err := c.spec.BuildV3Model()
	if err != nil {
		return fmt.Errorf("failed to build V3 model: %v", err)
	}

	if model.Model.Components == nil || model.Model.Components.SecuritySchemes == nil {
		return nil // No security schemes defined
	}

	c.authConfig.Schemes = []AuthScheme{} // Initialize or reset the schemes

	for pair := model.Model.Components.SecuritySchemes.First(); pair != nil; pair = pair.Next() {
		name := pair.Key()
		scheme := pair.Value()

		authScheme := AuthScheme{
			Name: name,
		}

		switch scheme.Type {
		case "http":
			if scheme.Scheme == "bearer" {
				authScheme.Method = AuthBearer
			} else if scheme.Scheme == "basic" {
				authScheme.Method = AuthBasic
			}
		case "apiKey":
			authScheme.Method = AuthApiKey
			authScheme.ApiKeyName = scheme.Name
			authScheme.ApiKeyIn = scheme.In
		case "oauth2":
			authScheme.Method = AuthOAuth2
			// You might want to store additional OAuth2 details here
		// Add more cases for other auth types as needed
		default:
			// Log unknown scheme type
			continue
		}

		c.authConfig.Schemes = append(c.authConfig.Schemes, authScheme)
	}

	return nil
}

// GetSupportedAuthSchemes returns information about the authentication schemes
// supported by the API as defined in the OpenAPI specification.
func (c *Client) GetSupportedAuthSchemes() []AuthSchemeInfo {
	var schemes []AuthSchemeInfo

	for _, scheme := range c.authConfig.Schemes {
		info := AuthSchemeInfo{
			Name: scheme.Name,
		}

		switch scheme.Method {
		case AuthBearer:
			info.Type = "HTTP Bearer"
			info.Description = "Use a Bearer token for authentication"
		case AuthBasic:
			info.Type = "HTTP Basic"
			info.Description = "Use Basic authentication with username and password"
		case AuthApiKey:
			info.Type = "API Key"
			info.Description = "Use an API key for authentication"
			info.In = scheme.ApiKeyIn
			info.KeyName = scheme.ApiKeyName
		case AuthOAuth2:
			info.Type = "OAuth2"
			info.Description = "Use OAuth2 for authentication"
		// Add more cases as needed
		default:
			info.Type = "Unknown"
			info.Description = "Unknown authentication type"
		}

		schemes = append(schemes, info)
	}

	return schemes
}

func (c *Client) AsTool(operations ...string) ([]llms.Tool, error) {
	if len(operations) == 0 {
		return nil, fmt.Errorf("at least one operation must be specified")
	}

	var tools []llms.Tool

	for _, operationID := range operations {
		operation, path, method, err := c.findOperation(operationID)
		if err != nil {
			return nil, fmt.Errorf("error finding operation %s: %w", operationID, err)
		}

		functionDef := &llms.FunctionDefinition{
			Name:        operationID,
			Description: c.getOperationDescription(operation, method, path),
			Parameters:  c.buildParametersSchema(operation),
		}

		tool := llms.Tool{
			Type:     "function",
			Function: functionDef,
		}

		tools = append(tools, tool)
	}

	return tools, nil
}

func (c *Client) getOperationDescription(operation *v3.Operation, method, path string) string {
	if operation.Description != "" {
		return operation.Description
	}

	if operation.Summary != "" {
		return operation.Summary
	}
	return fmt.Sprintf("%s %s", method, path)
}

func (c *Client) buildParametersSchema(operation *v3.Operation) map[string]interface{} {
	properties := make(map[string]interface{})
	required := []string{}

	for _, param := range operation.Parameters {
		properties[param.Name] = c.SchemaToMap(param.Schema.Schema())
		if param.Required == nil {
			continue
		}
		if *param.Required {
			required = append(required, param.Name)
		}
	}

	if operation.RequestBody != nil && operation.RequestBody.Content != nil {
		if content, ok := operation.RequestBody.Content.Get("application/json"); ok {
			properties["body"] = c.SchemaToMap(content.Schema.Schema())
			if operation != nil {
				if *operation.RequestBody.Required {
					required = append(required, "body")
				}
			}
		}
	}

	r := map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}

	// asJson, _ := json.MarshalIndent(r, "", "  ")
	// fmt.Println("======= TOOL SCHEMA =======")
	// fmt.Println(string(asJson))
	// fmt.Println("======= END  SCHEMA =======")

	return r
}

func (c *Client) SchemaToMap(schema *base.Schema) map[string]interface{} {
	if schema == nil {
		return nil
	}

	result := map[string]interface{}{}

	if len(schema.Type) > 0 {
		result["type"] = schema.Type[0]
	}

	if schema.Description != "" {
		result["description"] = schema.Description
	}

	if len(schema.Enum) > 0 {
		result["enum"] = schema.Enum
	}

	// Handle Items
	if schema.Items != nil && schema.Items.A != nil {
		result["items"] = c.SchemaToMap(schema.Items.A.Schema())
	}

	// Handle Properties
	if schema.Properties != nil && schema.Properties.Len() > 0 {
		properties := make(map[string]interface{})
		for pair := schema.Properties.First(); pair != nil; pair = pair.Next() {
			properties[pair.Key()] = c.SchemaToMap(pair.Value().Schema())
		}
		if len(properties) > 0 {
			result["properties"] = properties
		}
	}

	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}

	// Handle additional properties
	if schema.AdditionalProperties != nil {
		if schema.AdditionalProperties.B {
			result["additionalProperties"] = true
		} else if schema.AdditionalProperties.A != nil {
			result["additionalProperties"] = c.SchemaToMap(schema.AdditionalProperties.A.Schema())
		}
	}

	// Handle allOf, oneOf, anyOf
	if len(schema.AllOf) > 0 {
		allOf := make([]interface{}, len(schema.AllOf))
		for i, s := range schema.AllOf {
			allOf[i] = c.SchemaToMap(s.Schema())
		}
		result["allOf"] = allOf
	}

	if len(schema.OneOf) > 0 {
		oneOf := make([]interface{}, len(schema.OneOf))
		for i, s := range schema.OneOf {
			oneOf[i] = c.SchemaToMap(s.Schema())
		}
		result["oneOf"] = oneOf
	}

	if len(schema.AnyOf) > 0 {
		anyOf := make([]interface{}, len(schema.AnyOf))
		for i, s := range schema.AnyOf {
			anyOf[i] = c.SchemaToMap(s.Schema())
		}
		result["anyOf"] = anyOf
	}

	// Add other fields as needed (e.g., format, default, etc.)
	if schema.Format != "" {
		result["format"] = schema.Format
	}

	return result
}
