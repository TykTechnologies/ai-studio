package universalclient

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCallOperation(t *testing.T) {
	// Load the OpenAPI 3.0 definition
	specBytes, err := os.ReadFile("testdata/petstore.json")
	require.NoError(t, err)

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/pet/findByStatus", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		assert.Equal(t, "available", r.URL.Query().Get("status"))

		// Mock response
		pets := []map[string]interface{}{
			{
				"id":     1,
				"name":   "doggie",
				"status": "available",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pets)
	}))
	defer server.Close()

	// Create the client
	client, err := NewClient(specBytes, server.URL+"/v2")
	require.NoError(t, err)

	// Call the operation
	result, err := client.CallOperation(
		"findPetsByStatus",
		map[string][]string{"status": {"available"}},
		nil,
		nil,
	)

	// Assert the results
	require.NoError(t, err)
	pets, ok := result.([]interface{})
	require.True(t, ok)
	assert.Len(t, pets, 1)

	pet, ok := pets[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, float64(1), pet["id"])
	assert.Equal(t, "doggie", pet["name"])
	assert.Equal(t, "available", pet["status"])
}

func TestClientCallOperationLive(t *testing.T) {
	// Skip this test in CI environments or when running short tests
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("Skipping live API test in CI environment or short mode")
	}

	// Load the OpenAPI 3.0 definition
	specBytes, err := os.ReadFile("testdata/petstore.json")
	require.NoError(t, err)

	// Create the client
	// Note: We're not specifying a base URL, so it will use the one from the spec
	client, err := NewClient(specBytes, "")
	require.NoError(t, err)

	// Call the operation
	result, err := client.CallOperation(
		"findPetsByStatus",
		map[string][]string{"status": {"available"}},
		nil,
		nil,
	)

	// Assert the results
	require.NoError(t, err)
	require.NotNil(t, result)

	pets, ok := result.([]interface{})
	require.True(t, ok, "Expected result to be a slice of interfaces")
	require.NotEmpty(t, pets, "Expected at least one pet in the result")

	// Check the structure of the first pet
	pet, ok := pets[0].(map[string]interface{})
	require.True(t, ok, "Expected pet to be a map[string]interface{}")

	// Assert that the pet has the expected fields
	assert.Contains(t, pet, "id")
	assert.Contains(t, pet, "name")
	assert.Contains(t, pet, "status")

	// Assert that the status is "available"
	assert.Equal(t, "available", pet["status"])

	// fmt.Println(pet)
}

func TestClientCallOperationLiveWithJSONResponse(t *testing.T) {
	// Skip this test in CI environments or when running short tests
	if os.Getenv("CI") != "" || testing.Short() {
		t.Skip("Skipping live API test in CI environment or short mode")
	}

	// Load the OpenAPI 3.0 definition
	specBytes, err := os.ReadFile("testdata/petstore.json")
	require.NoError(t, err)

	// Create the client with ResponseFormatJSON option
	client, err := NewClient(specBytes, "", WithResponseFormat(ResponseFormatJSON))
	require.NoError(t, err)

	// Call the operation
	result, err := client.CallOperation(
		"findPetsByStatus",
		map[string][]string{"status": {"available"}},
		nil,
		nil,
	)

	// Assert the results
	require.NoError(t, err)
	require.NotNil(t, result)

	// Check that the result is a string (raw JSON)
	jsonStr, ok := result.(string)
	require.True(t, ok, "Expected result to be a string (raw JSON)")

	// Unmarshal the JSON string to verify its structure
	var pets []map[string]interface{}
	err = json.Unmarshal([]byte(jsonStr), &pets)
	require.NoError(t, err, "Failed to unmarshal JSON response")

	require.NotEmpty(t, pets, "Expected at least one pet in the result")

	// Check the structure of the first pet
	pet := pets[0]

	// Assert that the pet has the expected fields
	assert.Contains(t, pet, "id")
	assert.Contains(t, pet, "name")
	assert.Contains(t, pet, "status")

	// Assert that the status is "available"
	assert.Equal(t, "available", pet["status"])

	// fmt.Println("Raw JSON response:", jsonStr)
	// fmt.Println("First pet:", pet)
}

func TestListOperations(t *testing.T) {
	// Load the OpenAPI 3.0 definition
	specBytes, err := os.ReadFile("testdata/petstore.json")
	require.NoError(t, err)

	// Create the client
	client, err := NewClient(specBytes, "")
	require.NoError(t, err)

	// List the operations
	operations, err := client.ListOperations()
	require.NoError(t, err)

	// Assert the results
	assert.Contains(t, operations, "addPet")
	assert.Contains(t, operations, "deletePet")
	assert.Contains(t, operations, "findPetsByStatus")
	assert.Contains(t, operations, "findPetsByTags")
	assert.Contains(t, operations, "getPetById")
	assert.Contains(t, operations, "updatePet")
}

func TestAuthSchemes(t *testing.T) {
	// Define a basic OpenAPI spec with different auth schemes
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"servers": [{"url": "https://api.example.com/v1"}],
		"paths": {
			"/bearer": {
				"get": {
					"operationId": "bearerAuth",
					"security": [{"bearerAuth": []}]
				}
			},
			"/basic": {
				"get": {
					"operationId": "basicAuth",
					"security": [{"basicAuth": []}]
				}
			},
			"/apikey": {
				"get": {
					"operationId": "apiKeyAuth",
					"security": [{"apiKeyAuth": []}]
				}
			}
		},
		"components": {
			"securitySchemes": {
				"bearerAuth": {
					"type": "http",
					"scheme": "bearer"
				},
				"basicAuth": {
					"type": "http",
					"scheme": "basic"
				},
				"apiKeyAuth": {
					"type": "apiKey",
					"in": "header",
					"name": "X-API-Key"
				}
			}
		}
	}`

	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/bearer":
			if r.Header.Get("Authorization") != "Bearer test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		case "/basic":
			auth := r.Header.Get("Authorization")
			if auth != "Basic "+base64.StdEncoding.EncodeToString([]byte("testuser:testpass")) {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		case "/apikey":
			if r.Header.Get("X-API-Key") != "test-api-key" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	}))
	defer server.Close()

	// Create the client with ResponseFormatJSON
	client, err := NewClient([]byte(specJSON), server.URL, WithResponseFormat(ResponseFormatJSON))
	require.NoError(t, err)

	// Helper function to check the response
	checkResponse := func(t *testing.T, result interface{}, err error) {
		t.Helper()
		require.NoError(t, err)
		jsonStr, ok := result.(string)
		require.True(t, ok, "Expected result to be a string")

		var responseMap map[string]interface{}
		err = json.Unmarshal([]byte(jsonStr), &responseMap)
		require.NoError(t, err, "Failed to unmarshal JSON response")

		assert.Equal(t, map[string]interface{}{"status": "success"}, responseMap)
	}

	t.Run("BearerAuth", func(t *testing.T) {
		client.authConfig.Schemes = []AuthScheme{{
			Method: AuthBearer,
			Name:   "bearerAuth",
			Token:  "test-token",
		}}

		result, err := client.CallOperation("bearerAuth", nil, nil, nil)
		checkResponse(t, result, err)
	})

	t.Run("BasicAuth", func(t *testing.T) {
		client.authConfig.Schemes = []AuthScheme{{
			Method:   AuthBasic,
			Name:     "basicAuth",
			Username: "testuser",
			Password: "testpass",
		}}

		result, err := client.CallOperation("basicAuth", nil, nil, nil)
		checkResponse(t, result, err)
	})

	t.Run("ApiKeyAuth", func(t *testing.T) {
		client.authConfig.Schemes = []AuthScheme{{
			Method:     AuthApiKey,
			Name:       "apiKeyAuth",
			Token:      "test-api-key",
			ApiKeyName: "X-API-Key",
			ApiKeyIn:   "header",
		}}

		result, err := client.CallOperation("apiKeyAuth", nil, nil, nil)
		checkResponse(t, result, err)
	})

	t.Run("NoAuth", func(t *testing.T) {
		client.authConfig.Schemes = []AuthScheme{}

		_, err := client.CallOperation("bearerAuth", nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})

	t.Run("WrongAuth", func(t *testing.T) {
		client.authConfig.Schemes = []AuthScheme{{
			Method: AuthBearer,
			Name:   "bearerAuth",
			Token:  "wrong-token",
		}}

		_, err := client.CallOperation("bearerAuth", nil, nil, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "401")
	})
}

func TestGetSupportedAuthSchemes(t *testing.T) {
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"servers": [{"url": "https://api.example.com/v1"}],
		"components": {
			"securitySchemes": {
				"bearerAuth": {
					"type": "http",
					"scheme": "bearer"
				},
				"basicAuth": {
					"type": "http",
					"scheme": "basic"
				},
				"apiKeyAuth": {
					"type": "apiKey",
					"in": "header",
					"name": "X-API-Key"
				}
			}
		}
	}`

	client, err := NewClient([]byte(specJSON), "http://example.com")
	require.NoError(t, err)

	schemes := client.GetSupportedAuthSchemes()
	assert.Len(t, schemes, 3)

	expectedSchemes := map[string]AuthSchemeInfo{
		"bearerAuth": {
			Name:        "bearerAuth",
			Type:        "HTTP Bearer",
			Description: "Use a Bearer token for authentication",
		},
		"basicAuth": {
			Name:        "basicAuth",
			Type:        "HTTP Basic",
			Description: "Use Basic authentication with username and password",
		},
		"apiKeyAuth": {
			Name:        "apiKeyAuth",
			Type:        "API Key",
			Description: "Use an API key for authentication",
			In:          "header",
			KeyName:     "X-API-Key",
		},
	}

	for _, scheme := range schemes {
		expected, ok := expectedSchemes[scheme.Name]
		assert.True(t, ok, "Unexpected scheme: %s", scheme.Name)
		assert.Equal(t, expected, scheme)
	}
}

func TestValidateSpec(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr string
	}{
		{
			name: "Valid Spec with Bearer Auth",
			spec: `{
                "openapi": "3.0.0",
                "info": {"title": "Test API", "version": "1.0.0"},
                "servers": [{"url": "https://api.example.com/v1"}],
                "paths": {
                    "/test": {
                        "get": {
                            "operationId": "testOperation"
                        }
                    }
                },
                "components": {
                    "securitySchemes": {
                        "bearerAuth": {
                            "type": "http",
                            "scheme": "bearer"
                        }
                    }
                }
            }`,
			wantErr: "",
		},
		{
			name: "Valid Spec with Multiple Auth Types",
			spec: `{
                "openapi": "3.0.0",
                "info": {"title": "Test API", "version": "1.0.0"},
                "servers": [{"url": "https://api.example.com/v1"}],
                "paths": {
                    "/test": {
                        "get": {
                            "operationId": "testOperation"
                        }
                    }
                },
                "components": {
                    "securitySchemes": {
                        "bearerAuth": {
                            "type": "http",
                            "scheme": "bearer"
                        },
                        "oauth2Auth": {
                            "type": "oauth2",
                            "flows": {
                                "authorizationCode": {
                                    "authorizationUrl": "https://example.com/oauth/authorize",
                                    "tokenUrl": "https://example.com/oauth/token",
                                    "scopes": {
                                        "write:pets": "modify pets in your account",
                                        "read:pets": "read your pets"
                                    }
                                }
                            }
                        }
                    }
                }
            }`,
			wantErr: "",
		},
		{
			name: "Missing Servers",
			spec: `{
                "openapi": "3.0.0",
                "info": {"title": "Test API", "version": "1.0.0"}
            }`,
			wantErr: "specification must have at least one valid servers entry",
		},
		{
			name: "Missing SecuritySchemes",
			spec: `{
                "openapi": "3.0.0",
                "info": {"title": "Test API", "version": "1.0.0"},
                "servers": [{"url": "https://api.example.com/v1"}]
            }`,
			wantErr: "specification must have at least one SecuritySchema entry",
		},
		{
			name: "Only Unsupported Auth Type",
			spec: `{
                "openapi": "3.0.0",
                "info": {"title": "Test API", "version": "1.0.0"},
                "servers": [{"url": "https://api.example.com/v1"}],
                "components": {
                    "securitySchemes": {
                        "oauth2": {
                            "type": "oauth2",
                            "flows": {
                                "authorizationCode": {
                                    "authorizationUrl": "https://example.com/oauth/authorize",
                                    "tokenUrl": "https://example.com/oauth/token",
                                    "scopes": {
                                        "write:pets": "modify pets in your account",
                                        "read:pets": "read your pets"
                                    }
                                }
                            }
                        }
                    }
                }
            }`,
			wantErr: "specification must have at least one supported authentication type (apiKey, bearer, or basic)",
		},
		{
			name: "Missing OperationId",
			spec: `{
                "openapi": "3.0.0",
                "info": {"title": "Test API", "version": "1.0.0"},
                "servers": [{"url": "https://api.example.com/v1"}],
                "paths": {
                    "/test": {
                        "get": {}
                    }
                },
                "components": {
                    "securitySchemes": {
                        "bearerAuth": {
                            "type": "http",
                            "scheme": "bearer"
                        }
                    }
                }
            }`,
			wantErr: "all operations must have an operationID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := libopenapi.NewDocument([]byte(tt.spec))
			require.NoError(t, err)

			err = validateSpec(doc)

			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

// func TestGetOperationInputs(t *testing.T) {
// 	// Load the OpenAPI 3.0 definition
// 	specBytes, err := os.ReadFile("testdata/petstore.json")
// 	require.NoError(t, err)

// 	// Create the client
// 	client, err := NewClient(specBytes, "")
// 	require.NoError(t, err)

// 	operationId := "createUser"
// 	inputs, err := client.GetOperationInputs(operationId)
// 	require.NoError(t, err)

// 	fmt.Printf("Inputs for operation '%s':\n", operationId)

// 	fmt.Println("Path Parameters:")
// 	for _, param := range inputs.PathParams {
// 		fmt.Printf("- %s (Required: %v): %s\n", param.Name, param.Required, param.Description)
// 	}

// 	fmt.Println("Query Parameters:")
// 	for _, param := range inputs.QueryParams {
// 		fmt.Printf("- %s (Required: %v): %s\n", param.Name, param.Required, param.Description)
// 	}

// 	if inputs.RequestBody != nil {
// 		fmt.Printf("Request Body (Required: %v):\n", inputs.RequestBody.Required)
// 		fmt.Printf("- Content Type: %s\n", inputs.RequestBody.ContentType)
// 		fmt.Printf("- Description: %s\n", inputs.RequestBody.Description)
// 		x, _ := json.MarshalIndent(client.SchemaToMap(inputs.RequestBody.Schema), "", "  ")
// 		fmt.Printf("- Schema:\n%s\n", x)
// 		// You might want to add more detailed information about the schema here
// 	}
// }

// func TestScratch(t *testing.T) {
// 	if os.Getenv("CI") != "" || testing.Short() {
// 		t.Skip("Skipping live API test in CI environment or short mode")
// 	}

// 	// Load the OpenAPI 3.0 definition
// 	specBytes, err := os.ReadFile("testdata/jira.json")
// 	require.NoError(t, err)

// 	// Create the client
// 	client, err := NewClient(specBytes, "", WithResponseFormat(ResponseFormatJSON), WithAuth("apiKey", ""))
// 	require.NoError(t, err)

// 	// List the operations
// 	operations, err := client.ListOperations()
// 	require.NoError(t, err)

// 	fmt.Println("OPERATIONS:")
// 	for i, operation := range operations {
// 		fmt.Printf("%v. %s\n", i, operation)
// 	}

// 	// dat, err := client.AsTool(operations[3])
// 	// require.NoError(t, err)

// 	// jsonStr, err := json.MarshalIndent(dat, "", "  ")
// 	// require.NoError(t, err)
// 	// fmt.Println(string(jsonStr))

// 	// params := map[string][]string{
// 	// 	"city":    []string{"Auckland"},
// 	// 	"country": []string{"New Zealand"},
// 	// 	//"key":     []string{"060c284fb6104a00876a0df072682347"},
// 	// }

// 	// fmt.Println(operations[3])
// 	// result, err := client.CallOperation("Returnsadailyforecast-GivenLat/Lon.", params, map[string]interface{}{}, map[string][]string{})
// 	// require.NoError(t, err)
// 	// fmt.Println(result)

// 	// operationId := "addPet"
// 	// inputs, err := client.GetOperationInputs(operationId)
// 	// require.NoError(t, err)

// 	// fmt.Printf("Inputs for operation '%s':\n", operationId)
// 	// fmt.Println("Path Parameters:")
// 	// for _, param := range inputs.PathParams {
// 	// 	fmt.Printf("- %s (Required: %v): %s\n", param.Name, param.Required, param.Description)
// 	// }

// 	// fmt.Println("Query Parameters:")
// 	// for _, param := range inputs.QueryParams {
// 	// 	fmt.Printf("- %s (Required: %v): %s\n", param.Name, param.Required, param.Description)
// 	// }

// 	// if inputs.RequestBody != nil {
// 	// 	fmt.Printf("Request Body (Required: %v):\n", inputs.RequestBody.Required)
// 	// 	fmt.Printf("- Content Type: %s\n", inputs.RequestBody.ContentType)
// 	// 	fmt.Printf("- Description: %s\n", inputs.RequestBody.Description)

// 	// 	// You might want to add more detailed information about the schema here
// 	// }

// 	// result, err := client.CallOperation(
// 	// 	operationId,
// 	// 	nil,
// 	// 	nil,
// 	// 	nil,
// 	// )

// 	// require.NoError(t, err)
// 	// require.NotNil(t, result)

// 	// fmt.Println("RESULT:")
// 	// fmt.Println(result)

// }
