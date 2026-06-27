package exporter

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type VerifyResult struct {
	SchemaVersion string       `json:"schemaVersion"`
	VerifiedAt    time.Time    `json:"verifiedAt"`
	ManifestPath  string       `json:"manifestPath"`
	Valid         bool         `json:"valid"`
	Files         []VerifyFile `json:"files"`
	Errors        []string     `json:"errors,omitempty"`
}

type VerifyFile struct {
	Path              string   `json:"path"`
	Valid             bool     `json:"valid"`
	ExpectedSHA256    string   `json:"expectedSha256"`
	ActualSHA256      string   `json:"actualSha256,omitempty"`
	ExpectedSizeBytes int64    `json:"expectedSizeBytes"`
	ActualSizeBytes   int64    `json:"actualSizeBytes,omitempty"`
	ExpectedRowCount  int      `json:"expectedRowCount"`
	ActualRowCount    int      `json:"actualRowCount,omitempty"`
	Errors            []string `json:"errors,omitempty"`
}

func VerifyPublicExport(exportDir string) (VerifyResult, error) {
	if exportDir == "" {
		return VerifyResult{}, errors.New("export directory is required")
	}
	manifestPath := filepath.Join(exportDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return VerifyResult{}, err
	}
	var manifest ExportManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return VerifyResult{}, fmt.Errorf("decode manifest: %w", err)
	}
	if manifest.SchemaVersion != "registry.export_manifest.v1" {
		return VerifyResult{}, fmt.Errorf("unsupported manifest schema version %q", manifest.SchemaVersion)
	}
	result := VerifyResult{
		SchemaVersion: "registry.export_verify.v1",
		VerifiedAt:    time.Now().UTC(),
		ManifestPath:  manifestPath,
		Valid:         true,
	}
	seenPaths := make(map[string]struct{}, len(manifest.Files))
	for _, expected := range manifest.Files {
		fileResult := VerifyFile{
			Path:              expected.Path,
			Valid:             true,
			ExpectedSHA256:    expected.SHA256,
			ExpectedSizeBytes: expected.SizeBytes,
			ExpectedRowCount:  expected.RowCount,
		}
		if _, ok := seenPaths[expected.Path]; ok {
			fileResult.Valid = false
			fileResult.Errors = append(fileResult.Errors, "duplicate manifest file path")
		}
		seenPaths[expected.Path] = struct{}{}
		if expected.ContentType != contentTypeForExportFile(expected.Path) {
			fileResult.Valid = false
			fileResult.Errors = append(fileResult.Errors, fmt.Sprintf("content type mismatch: expected %s for path", contentTypeForExportFile(expected.Path)))
		}
		path, err := safeExportFilePath(exportDir, expected.Path)
		if err != nil {
			fileResult.Valid = false
			fileResult.Errors = append(fileResult.Errors, err.Error())
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", expected.Path, err))
			result.Valid = false
			result.Files = append(result.Files, fileResult)
			continue
		}
		actual, err := inspectExportFile(path)
		if err != nil {
			fileResult.Valid = false
			fileResult.Errors = append(fileResult.Errors, err.Error())
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", expected.Path, err))
			result.Valid = false
			result.Files = append(result.Files, fileResult)
			continue
		}
		fileResult.ActualSHA256 = actual.sha256
		fileResult.ActualSizeBytes = actual.sizeBytes
		fileResult.ActualRowCount = actual.rowCount
		if expected.SHA256 != actual.sha256 {
			fileResult.Valid = false
			fileResult.Errors = append(fileResult.Errors, "sha256 mismatch")
		}
		if expected.SizeBytes != actual.sizeBytes {
			fileResult.Valid = false
			fileResult.Errors = append(fileResult.Errors, "size mismatch")
		}
		if expected.RowCount != actual.rowCount {
			fileResult.Valid = false
			fileResult.Errors = append(fileResult.Errors, "row count mismatch")
		}
		if shapeErrors := validateExportFileShape(path, expected.Path); len(shapeErrors) > 0 {
			fileResult.Valid = false
			fileResult.Errors = append(fileResult.Errors, shapeErrors...)
		}
		if !fileResult.Valid {
			result.Valid = false
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %s", expected.Path, strings.Join(fileResult.Errors, "; ")))
		}
		result.Files = append(result.Files, fileResult)
	}
	return result, nil
}

type inspectedExportFile struct {
	sha256    string
	sizeBytes int64
	rowCount  int
}

func safeExportFilePath(root, name string) (string, error) {
	if name == "" {
		return "", errors.New("unsafe export file path: empty path")
	}
	if strings.Contains(name, "\\") {
		return "", fmt.Errorf("unsafe export file path %q: backslash paths are not allowed", name)
	}
	if filepath.IsAbs(name) || path.IsAbs(name) {
		return "", fmt.Errorf("unsafe export file path %q: absolute paths are not allowed", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe export file path %q: parent traversal is not allowed", name)
	}
	if clean != path.Base(clean) {
		return "", fmt.Errorf("unsafe export file path %q: nested paths are not allowed", name)
	}
	return filepath.Join(root, clean), nil
}

func inspectExportFile(path string) (inspectedExportFile, error) {
	file, err := os.Open(path)
	if err != nil {
		return inspectedExportFile{}, err
	}
	defer file.Close()
	hash := sha256.New()
	counter := newLineCounter(filepath.Ext(path) == ".jsonl")
	writer := io.MultiWriter(hash, counter)
	size, err := io.Copy(writer, file)
	if err != nil {
		return inspectedExportFile{}, err
	}
	return inspectedExportFile{
		sha256:    hex.EncodeToString(hash.Sum(nil)),
		sizeBytes: size,
		rowCount:  counter.rows(size),
	}, nil
}

func validateExportFileShape(path, exportName string) []string {
	if filepath.Ext(exportName) != ".jsonl" {
		return nil
	}
	required := requiredExportFields(exportName)
	if len(required) == 0 {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return []string{err.Error()}
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var errs []string
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			errs = append(errs, fmt.Sprintf("line %d: empty JSONL row", line))
			continue
		}
		var row map[string]json.RawMessage
		if err := json.Unmarshal([]byte(text), &row); err != nil {
			errs = append(errs, fmt.Sprintf("line %d: invalid JSON: %v", line, err))
			continue
		}
		for _, field := range required {
			value, ok := row[field]
			if !ok || emptyJSONValue(value) {
				errs = append(errs, fmt.Sprintf("line %d: missing required field %q", line, field))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		errs = append(errs, err.Error())
	}
	return errs
}

func requiredExportFields(name string) []string {
	switch name {
	case "entities.jsonl":
		return []string{"id", "slug", "entityType", "name", "environment"}
	case "killmails.jsonl":
		return []string{"id", "environment", "occurredAt", "sourceIds"}
	case "sources.jsonl":
		return []string{"id", "kind", "title", "locator"}
	case "events.jsonl":
		return []string{"id", "kind", "environment", "occurredAt"}
	case "sui_objects.jsonl":
		return []string{"id", "objectId", "environment", "typeRepr", "observedAt"}
	default:
		return nil
	}
}

func emptyJSONValue(value json.RawMessage) bool {
	if len(value) == 0 || string(value) == "null" {
		return true
	}
	var decoded any
	if err := json.Unmarshal(value, &decoded); err != nil {
		return false
	}
	switch item := decoded.(type) {
	case string:
		return strings.TrimSpace(item) == ""
	case []any:
		return len(item) == 0
	default:
		return false
	}
}

type lineCounter struct {
	jsonl      bool
	lineCount  int
	lastByte   byte
	sawContent bool
}

func newLineCounter(jsonl bool) *lineCounter {
	return &lineCounter{jsonl: jsonl}
}

func (c *lineCounter) Write(data []byte) (int, error) {
	if len(data) > 0 {
		c.sawContent = true
		c.lastByte = data[len(data)-1]
	}
	if c.jsonl {
		c.lineCount += countByte(data, '\n')
	}
	return len(data), nil
}

func (c *lineCounter) rows(size int64) int {
	if !c.jsonl {
		if size == 0 {
			return 0
		}
		return 1
	}
	if c.sawContent && c.lastByte != '\n' {
		return c.lineCount + 1
	}
	return c.lineCount
}

func countByte(data []byte, target byte) int {
	count := 0
	for _, value := range data {
		if value == target {
			count++
		}
	}
	return count
}
