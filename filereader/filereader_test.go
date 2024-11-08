package filereader

import (
	"os"
	"strings"
	"testing"
)

func TestReadPDFFile(t *testing.T) {
	// Read the PDF file
	testFile := "testdata/sample.pdf"
	fileData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// Test the Read function
	text, err := Read(testFile, fileData)
	if err != nil {
		t.Fatalf("Failed to read PDF: %v", err)
	}

	// Check if the text contains expected content
	expectedContent := "Lorem ipsum dolor sit amet"
	if text == "" {
		t.Error("Expected non-empty text content")
	}
	if !strings.Contains(text, expectedContent) {
		t.Errorf("Expected text to contain '%s', got '%s'", expectedContent, text)
	}
}

func TestReadDOCXFile(t *testing.T) {
	// Read the DOCX file
	testFile := "testdata/sample.docx"
	fileData, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// Test the Read function
	text, err := Read(testFile, fileData)
	if err != nil {
		t.Fatalf("Failed to read DOCX: %v", err)
	}

	// Check if the text contains expected content
	expectedContent := "Lorem ipsum dolor sit amet"
	if text == "" {
		t.Error("Expected non-empty text content")
	}
	if !strings.Contains(text, expectedContent) {
		t.Errorf("Expected text to contain '%s', got '%s'", expectedContent, text)
	}
}
