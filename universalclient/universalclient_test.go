package universalclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/pb33f/libopenapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
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
		// Non longer enforcing sec scheme requirement
		// {
		// 	name: "Missing SecuritySchemes",
		// 	spec: `{
		//               "openapi": "3.0.0",
		//               "info": {"title": "Test API", "version": "1.0.0"},
		//               "servers": [{"url": "https://api.example.com/v1"}]
		//           }`,
		// 	wantErr: "specification must have at least one SecuritySchema entry",
		// },
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

func TestBuildParametersSchemaWithNilRequired(t *testing.T) {
	// Create a test OpenAPI spec with a RequestBody that has a nil Required field
	specJSON := `{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"servers": [
			{
				"url": "https://api.example.com"
			}
		],
		"paths": {
			"/test": {
				"post": {
					"operationId": "postTest",
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {
									"type": "object",
									"properties": {
										"message": {
											"type": "string"
										}
									}
								}
							}
						}
					},
					"responses": {
						"200": {
							"description": "OK"
						}
					}
				}
			}
		}
	}`

	client, err := NewClient([]byte(specJSON), "")
	assert.NoError(t, err)
	assert.NotNil(t, client)

	// Find the operation
	operation, _, _, err := client.findOperation("postTest")
	assert.NoError(t, err)
	assert.NotNil(t, operation)

	// Verify that the RequestBody.Required is nil
	assert.NotNil(t, operation.RequestBody)
	assert.Nil(t, operation.RequestBody.Required)

	// This should not panic
	schema := client.buildParametersSchema(operation)
	assert.NotNil(t, schema)
	assert.Contains(t, schema, "properties")
	assert.Contains(t, schema["properties"].(map[string]interface{}), "body")
}

func TestNoBodyForGetHeadOptionsRequests(t *testing.T) {
	// Define a basic OpenAPI spec with different HTTP methods
	specJSON := `{
		"openapi": "3.0.0",
		"info": {"title": "Test API", "version": "1.0.0"},
		"servers": [{"url": "https://api.example.com/v1"}],
		"paths": {
			"/test": {
				"get": {
					"operationId": "testGet"
				},
				"post": {
					"operationId": "testPost"
				},
				"put": {
					"operationId": "testPut"
				},
				"delete": {
					"operationId": "testDelete"
				},
				"head": {
					"operationId": "testHead"
				},
				"options": {
					"operationId": "testOptions"
				},
				"patch": {
					"operationId": "testPatch"
				}
			}
		}
	}`

	// Create a mock server that checks for request bodies
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Check if body is empty for GET, HEAD, OPTIONS
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
			if len(body) > 0 {
				t.Errorf("Request body should be empty for %s method, got: %s", r.Method, string(body))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		} else {
			// For other methods, check if body contains the expected payload
			if r.Header.Get("Content-Type") == "application/json" {
				var payload map[string]interface{}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Errorf("Failed to unmarshal request body: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				// Verify payload contains the expected data
				if val, ok := payload["test"]; !ok || val != "data" {
					t.Errorf("Expected payload with {\"test\":\"data\"}, got: %s", string(body))
					w.WriteHeader(http.StatusBadRequest)
					return
				}
			}
		}

		// Return success response
		w.WriteHeader(http.StatusOK)
		// Skip writing response body for HEAD requests
		if r.Method != "HEAD" {
			json.NewEncoder(w).Encode(map[string]string{"status": "success"})
		}
	}))
	defer server.Close()

	// Create the client
	client, err := NewClient([]byte(specJSON), server.URL, WithResponseFormat(ResponseFormatJSON))
	require.NoError(t, err)

	// Test payload
	payload := map[string]interface{}{"test": "data"}
	emptyPayload := map[string]interface{}{}

	// Test cases
	testCases := []struct {
		name        string
		operationId string
		method      string
		payload     map[string]interface{}
	}{
		{"GET with payload", "testGet", "GET", payload},
		{"GET with empty payload", "testGet", "GET", emptyPayload},
		{"POST with payload", "testPost", "POST", payload},
		{"PUT with payload", "testPut", "PUT", payload},
		{"DELETE with payload", "testDelete", "DELETE", payload},
		{"HEAD with payload", "testHead", "HEAD", payload},
		{"HEAD with empty payload", "testHead", "HEAD", emptyPayload},
		{"OPTIONS with payload", "testOptions", "OPTIONS", payload},
		{"OPTIONS with empty payload", "testOptions", "OPTIONS", emptyPayload},
		{"PATCH with payload", "testPatch", "PATCH", payload},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := client.CallOperation(tc.operationId, nil, tc.payload, nil)

			// All requests should succeed
			require.NoError(t, err, "Operation should not fail")

			// Skip response body validation for HEAD requests since they don't return a body
			if tc.method != "HEAD" {
				// Verify response
				jsonStr, ok := result.(string)
				require.True(t, ok, "Expected result to be a string")

				var responseMap map[string]interface{}
				err = json.Unmarshal([]byte(jsonStr), &responseMap)
				require.NoError(t, err, "Failed to unmarshal JSON response")

				assert.Equal(t, "success", responseMap["status"], "Expected success status")
			}
		})
	}
}

func decodeToUTF8(s string) (string, error) {
	// Step 1: Decode base64
	decodedBytes, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("base64 decoding failed: %v", err)
	}

	// Step 2 & 3: Convert to UTF-8
	// This example assumes the original encoding was Windows-1252 (a common encoding)
	// Replace this with the correct encoding if known
	reader := transform.NewReader(strings.NewReader(string(decodedBytes)), charmap.Windows1252.NewDecoder())
	utf8Bytes, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("conversion to UTF-8 failed: %v", err)
	}

	return string(utf8Bytes), nil
}

// func Test_GetOperations(t *testing.T) {
// 	// Load the OpenAPI 3.0 definition
// 	specBytes, err := os.ReadFile("testdata/zendesk.yaml")
// 	require.NoError(t, err)

// 	// Create the client
// 	client, err := NewClient(specBytes, "")
// 	require.NoError(t, err)

// 	// List the operations
// 	operations, err := client.ListOperations()
// 	require.NoError(t, err)

// 	for _, o := range operations {
// 		fmt.Println(o)
// 	}
// }
