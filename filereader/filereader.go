package filereader

import (
	"bytes"
	"fmt"
	"os"
	"strings"

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
		return ExtractPDFText(name, fileData)
	case ".odt":
		return ExtractMSFormats(name, fileData)
	case ".docx":
		return ExtractMSFormats(name, fileData)
	case ".rtf":
		return ExtractMSFormats(name, fileData)
	}

	return "", fmt.Errorf("unsupported file type: %s", ext)
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
