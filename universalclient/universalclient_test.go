package universalclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

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

func TestGetOperationInputs(t *testing.T) {
	// Load the OpenAPI 3.0 definition
	specBytes, err := os.ReadFile("testdata/petstore.json")
	require.NoError(t, err)

	// Create the client
	client, err := NewClient(specBytes, "")
	require.NoError(t, err)

	operationId := "createUser"
	inputs, err := client.GetOperationInputs(operationId)
	require.NoError(t, err)

	fmt.Printf("Inputs for operation '%s':\n", operationId)

	fmt.Println("Path Parameters:")
	for _, param := range inputs.PathParams {
		fmt.Printf("- %s (Required: %v): %s\n", param.Name, param.Required, param.Description)
	}

	fmt.Println("Query Parameters:")
	for _, param := range inputs.QueryParams {
		fmt.Printf("- %s (Required: %v): %s\n", param.Name, param.Required, param.Description)
	}

	if inputs.RequestBody != nil {
		fmt.Printf("Request Body (Required: %v):\n", inputs.RequestBody.Required)
		fmt.Printf("- Content Type: %s\n", inputs.RequestBody.ContentType)
		fmt.Printf("- Description: %s\n", inputs.RequestBody.Description)
		x, _ := json.MarshalIndent(client.SchemaToMap(inputs.RequestBody.Schema), "", "  ")
		fmt.Printf("- Schema:\n%s\n", x)
		// You might want to add more detailed information about the schema here
	}
}

// func TestScratch(t *testing.T) {
// 	if os.Getenv("CI") != "" || testing.Short() {
// 		t.Skip("Skipping live API test in CI environment or short mode")
// 	}

// 	// Load the OpenAPI 3.0 definition
// 	specBytes, err := os.ReadFile("testdata/petstore.json")
// 	require.NoError(t, err)

// 	// Create the client
// 	client, err := NewClient(specBytes, "https://httpbin.dmuth.org", WithResponseFormat(ResponseFormatJSON))
// 	require.NoError(t, err)

// 	// List the operations
// 	operations, err := client.ListOperations()
// 	require.NoError(t, err)

// 	fmt.Println("OPERATIONS:")
// 	for i, operation := range operations {
// 		fmt.Printf("%v. %s\n", i, operation)
// 	}

// 	dat, err := client.AsTool(operations[19])
// 	require.NoError(t, err)

// 	jsonStr, err := json.MarshalIndent(dat, "", "  ")
// 	require.NoError(t, err)
// 	fmt.Println(string(jsonStr))

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
