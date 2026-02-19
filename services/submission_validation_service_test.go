package services

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Minimal valid OpenAPI 3.0 spec
var validOASSpec = `
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
servers:
  - url: https://api.example.com/v1
paths:
  /users:
    get:
      operationId: listUsers
      summary: List all users
      responses:
        "200":
          description: OK
    post:
      operationId: createUser
      summary: Create a user
      responses:
        "201":
          description: Created
components:
  securitySchemes:
    ApiKeyAuth:
      type: apiKey
      in: header
      name: X-API-Key
`

// Spec missing servers entry
var noServersSpec = `
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
paths:
  /users:
    get:
      operationId: listUsers
      responses:
        "200":
          description: OK
`

// Spec with missing operationId
var missingOperationIDSpec = `
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /users:
    get:
      summary: List users
      responses:
        "200":
          description: OK
`

// Spec with no auth schemes (should be a warning, not an error)
var noAuthSpec = `
openapi: "3.0.0"
info:
  title: Test API
  version: "1.0"
servers:
  - url: https://api.example.com
paths:
  /health:
    get:
      operationId: healthCheck
      responses:
        "200":
          description: OK
`

func TestValidateOASSpec_ValidSpec(t *testing.T) {
	service := NewService(nil) // DB not needed for spec validation

	encoded := base64.StdEncoding.EncodeToString([]byte(validOASSpec))
	result, err := service.ValidateOASSpec(encoded)
	assert.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
	assert.NotNil(t, result.Extracted)
	assert.Len(t, result.Extracted.Operations, 2)
	assert.Contains(t, result.Extracted.Operations, "listUsers")
	assert.Contains(t, result.Extracted.Operations, "createUser")
	assert.Len(t, result.Extracted.AuthSchemes, 1)
	assert.Equal(t, "API Key", result.Extracted.AuthSchemes[0].Type)
}

func TestValidateOASSpec_InvalidBase64(t *testing.T) {
	service := NewService(nil)

	result, err := service.ValidateOASSpec("not-valid-base64!!!")
	assert.NoError(t, err) // returns result, not error
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "oas_spec", result.Errors[0].Field)
	assert.Contains(t, result.Errors[0].Message, "Failed to decode base64")
}

func TestValidateOASSpec_NoServers(t *testing.T) {
	service := NewService(nil)

	encoded := base64.StdEncoding.EncodeToString([]byte(noServersSpec))
	result, err := service.ValidateOASSpec(encoded)
	assert.NoError(t, err)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "servers", result.Errors[0].Field)
	assert.Contains(t, result.Errors[0].Message, "servers entry")
}

func TestValidateOASSpec_MissingOperationID(t *testing.T) {
	service := NewService(nil)

	encoded := base64.StdEncoding.EncodeToString([]byte(missingOperationIDSpec))
	result, err := service.ValidateOASSpec(encoded)
	assert.NoError(t, err)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Equal(t, "paths", result.Errors[0].Field)
	assert.Contains(t, result.Errors[0].Message, "operationID")
}

func TestValidateOASSpec_NoAuth_Warning(t *testing.T) {
	service := NewService(nil)

	encoded := base64.StdEncoding.EncodeToString([]byte(noAuthSpec))
	result, err := service.ValidateOASSpec(encoded)
	assert.NoError(t, err)
	assert.True(t, result.Valid)
	assert.Empty(t, result.Errors)
	// Should have a warning about no auth
	assert.NotEmpty(t, result.Warnings)
	found := false
	for _, w := range result.Warnings {
		if w.Field == "components.securitySchemes" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected warning about missing auth schemes")
}

func TestValidateOASSpec_InvalidYAML(t *testing.T) {
	service := NewService(nil)

	encoded := base64.StdEncoding.EncodeToString([]byte("this is not yaml: [[["))
	result, err := service.ValidateOASSpec(encoded)
	assert.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}

func TestValidateOASSpec_SpecTooLarge(t *testing.T) {
	service := NewService(nil)

	// Create a spec larger than 1MB
	largeSpec := make([]byte, 1024*1024+1)
	for i := range largeSpec {
		largeSpec[i] = 'x'
	}
	encoded := base64.StdEncoding.EncodeToString(largeSpec)

	result, err := service.ValidateOASSpec(encoded)
	assert.NoError(t, err)
	assert.False(t, result.Valid)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0].Message, "exceeds maximum")
}

func TestValidateOASSpec_OpenAPI2NotSupported(t *testing.T) {
	service := NewService(nil)

	swagger2Spec := `
swagger: "2.0"
info:
  title: Test API
  version: "1.0"
host: api.example.com
paths:
  /users:
    get:
      operationId: listUsers
      responses:
        200:
          description: OK
`
	encoded := base64.StdEncoding.EncodeToString([]byte(swagger2Spec))
	result, err := service.ValidateOASSpec(encoded)
	assert.NoError(t, err)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.Errors)
}
