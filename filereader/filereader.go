package filereader

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"log"

	"github.com/dslipak/pdf"
	"github.com/lu4p/cat"
)

var TextExtensions = []string{
	".txt", ".md", ".markdown", ".csv", ".tsv", // Text and Markdown
	".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".cs", ".rb", ".php", // Common programming languages
	".html", ".css", ".json", ".xml", ".yaml", ".yml", ".toml", // Web-related files
	".sh", ".bash", // Shell scripts
	".sql", // SQL files
}

func Read(name string, fileData []byte) (string, error) {
	parts := strings.Split(name, ".")
	ext := parts[len(parts)-1]

	dotExt := fmt.Sprintf(".%s", strings.ToLower(ext))

	for _, textExt := range TextExtensions {
		if dotExt == textExt {
			return string(fileData), nil
		}
	}

	switch dotExt {
	case ".pdf":
		return ExtractTextFromPDFUsingPopplerWithOptions(name, (10 * time.Second), fileData)
	case ".odt":
		return ExtractMSFormats(name, fileData)
	case ".docx":
		return ExtractMSFormats(name, fileData)
	case ".rtf":
		return ExtractMSFormats(name, fileData)
	}

	return "", fmt.Errorf("unsupported file type: %s", ext)
}

func ExtractTextFromPDFUsingPopplerWithOptions(fileName string, timeout time.Duration, data []byte) (string, error) {
	// First check if pdftotext is in PATH
	pdftotextPath, err := exec.LookPath("pdftotext")
	if err != nil {
		// If not in PATH, check the local directory
		localPdftotext := "./pdftotext"
		if _, err := os.Stat(localPdftotext); err == nil {
			pdftotextPath = localPdftotext
		} else {
			log.Println("WARNING: pdftotext not found in PATH or local directory, falling back to pdf package")
			return ExtractPDFText(fileName, data)
		}
	}

	// write data to pdfPath
	tempInput, err := os.CreateTemp("", "pdf_extract_*.pdf")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tempInput.Name())

	err = os.WriteFile(tempInput.Name(), data, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write to temporary file: %v", err)
	}

	// Check if the input file exists
	if _, err := os.Stat(tempInput.Name()); os.IsNotExist(err) {
		return "", fmt.Errorf("PDF file does not exist: %s", tempInput.Name())
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create a temporary file with random name
	tempFile, err := os.CreateTemp("", "pdf_extract_*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	tempFile.Close()

	// Create the pdftotext command with context using the found path
	cmd := exec.CommandContext(ctx, pdftotextPath, "-layout", "-nopgbrk", tempInput.Name(), tempFile.Name())

	// Capture both stdout and stderr
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Execute the command
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to extract text: %v - %s", err, stderr.String())
	}

	// Read the contents of the temporary file
	content, err := os.ReadFile(tempFile.Name())
	if err != nil {
		return "", fmt.Errorf("failed to read temporary file: %v", err)
	}

	return string(content), nil
}

func ExtractPDFText(fileName string, data []byte) (string, error) {
	// dump to file
	tmpFile, err := os.CreateTemp("/tmp", "pdf_extract-*.txt")
	if err != nil {
		return "", err
	}

	defer os.Remove(tmpFile.Name())

	os.WriteFile(tmpFile.Name(), data, 0644)
	defer tmpFile.Close()

	r, err := pdf.Open(tmpFile.Name())
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	b, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	buf.ReadFrom(b)

	return buf.String(), nil
}

func ExtractMSFormats(fileName string, data []byte) (string, error) {
	// dump to file
	tmpFile, err := os.CreateTemp("/tmp", "msdoc_extract-*.txt")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpFile.Name())

	os.WriteFile(tmpFile.Name(), data, 0644)
	defer tmpFile.Close()

	txt, err := cat.File(tmpFile.Name())
	if err != nil {
		return "", err
	}

	return txt, nil
}
