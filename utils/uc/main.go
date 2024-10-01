package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/TykTechnologies/midsommar/v2/universalclient"
)

func testFile(filename string) error {
	specBytes, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	_, err = universalclient.NewClient(specBytes, "")
	if err != nil {
		return fmt.Errorf("failed to parse OpenAPI spec: %v", err)
	}

	fmt.Println("File parsed successfully")
	return nil
}

func listOperations(filename string) error {
	specBytes, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	client, err := universalclient.NewClient(specBytes, "")
	if err != nil {
		return fmt.Errorf("failed to parse OpenAPI spec: %v", err)
	}

	operations, err := client.ListOperations()
	if err != nil {
		return fmt.Errorf("failed to list operations: %v", err)
	}

	fmt.Println("Operations:")
	for i, operation := range operations {
		fmt.Printf("%d. %s\n", i+1, operation)
	}

	return nil
}

func callOperation(filename, operationID string, params []string) error {
	specBytes, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %v", err)
	}

	client, err := universalclient.NewClient(specBytes, "", universalclient.WithResponseFormat(universalclient.ResponseFormatJSON))
	if err != nil {
		return fmt.Errorf("failed to parse OpenAPI spec: %v", err)
	}

	paramMap := make(map[string][]string)
	for _, param := range params {
		parts := strings.SplitN(param, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid parameter format: %s", param)
		}
		paramMap[parts[0]] = []string{parts[1]}
	}

	result, err := client.CallOperation(operationID, paramMap, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to call operation: %v", err)
	}

	fmt.Println("Result:")
	fmt.Println(result)

	return nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage:")
		fmt.Println("  test <filename>")
		fmt.Println("  list <filename>")
		fmt.Println("  call <filename> <operation> [paramname:value...]")
		os.Exit(1)
	}

	command := os.Args[1]
	filename := os.Args[2]

	var err error

	switch command {
	case "test":
		err = testFile(filename)
	case "list":
		err = listOperations(filename)
	case "call":
		if len(os.Args) < 4 {
			fmt.Println("Usage: call <filename> <operation> [paramname:value...]")
			os.Exit(1)
		}
		operationID := os.Args[3]
		params := os.Args[4:]
		err = callOperation(filename, operationID, params)
	default:
		fmt.Printf("Unknown command: %s\n", command)
		os.Exit(1)
	}

	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
}
