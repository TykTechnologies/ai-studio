package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

type PredictRequest struct {
	Instances []map[string]interface{} `json:"instances"`
}

type PredictResponse struct {
	Predictions      []string `json:"predictions"`
	DeployedModelID  string   `json:"deployedModelId"`
	Model            string   `json:"model"`
	ModelDisplayName string   `json:"modelDisplayName"`
	ModelVersionID   string   `json:"modelVersionId"`
}

func main() {
	// Command-line flags
	projectID := flag.String("project", "", "Google Cloud Project ID (required)")
	location := flag.String("location", "us-central1", "Google Cloud region")
	modelName := flag.String("model", "", "Vertex AI model name (required)")
	prompt := flag.String("prompt", "", "Prompt to send to the model (required)")
	apiEndpoint := flag.String("api-endpoint", "", "Vertex AI API endpoint (optional)")
	apiKey := flag.String("api-key", "", "API key for AI Gateway (required)")

	flag.Parse()

	if *projectID == "" || *modelName == "" || *prompt == "" || *apiKey == "" {
		fmt.Println("Error: project, model, prompt, and api-key are required")
		flag.Usage()
		os.Exit(1)
	}

	if *apiEndpoint == "" {
		log.Fatal("Error: api-endpoint is required")
		*apiEndpoint = fmt.Sprintf("https://%s-aiplatform.googleapis.com", *location)
	}

	url := fmt.Sprintf("%s/v1/projects/%s/locations/%s/endpoints/%s:predict", *apiEndpoint, *projectID, *location, *modelName)

	requestBody := PredictRequest{
		Instances: []map[string]interface{}{
			{"prompt": *prompt},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		log.Fatalf("Error marshaling request body: %v", err)
	}

	fmt.Println("POSTING TO:", url)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Fatalf("Error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+*apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		fmt.Println("Request:", resp.StatusCode)
		fmt.Println("Headers", resp.Header)
		log.Fatalf("Error response from API: %s", body)
	}

	var predictResponse PredictResponse
	err = json.Unmarshal(body, &predictResponse)
	if err != nil {
		fmt.Println(string(body))
		log.Fatalf("Error unmarshaling response: %v", err)
	}

	fmt.Println("Response:")
	for _, prediction := range predictResponse.Predictions {
		fmt.Println(prediction)
	}
}
