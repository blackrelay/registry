package staticclient

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ResourceIndex []ResourceIndexEntry

type ResourceIndexEntry struct {
	ResourcePath string `json:"resourcePath"`
	HashPath     string `json:"hashPath"`
	ContentHash  string `json:"contentHash,omitempty"`
	IndexSize    int64  `json:"indexSize,omitempty"`
	PackedSize   int64  `json:"packedSize,omitempty"`
}

type StaticResourceEvidence struct {
	ResourcePath string `json:"resourcePath"`
	HashPath     string `json:"hashPath"`
	Path         string `json:"path"`
	SHA256       string `json:"sha256"`
	SizeBytes    int64  `json:"sizeBytes"`
	IndexSize    int64  `json:"indexSize,omitempty"`
	PackedSize   int64  `json:"packedSize,omitempty"`
}

type StaticResourceDiscovery struct {
	Kind         string                 `json:"kind"`
	Priority     int                    `json:"priority"`
	ResourcePath string                 `json:"resourcePath"`
	Evidence     StaticResourceEvidence `json:"evidence"`
}

func ParseResourceIndex(r io.Reader) (ResourceIndex, error) {
	scanner := bufio.NewScanner(r)
	buffer := make([]byte, 0, 1024*1024)
	scanner.Buffer(buffer, 16*1024*1024)
	var entries ResourceIndex
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		entry := ResourceIndexEntry{
			ResourcePath: strings.TrimSpace(parts[0]),
			HashPath:     filepath.ToSlash(strings.TrimSpace(parts[1])),
		}
		if len(parts) > 2 {
			entry.ContentHash = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 && strings.TrimSpace(parts[3]) != "" {
			value, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse resource index line %d index size: %w", lineNumber, err)
			}
			entry.IndexSize = value
		}
		if len(parts) > 4 && strings.TrimSpace(parts[4]) != "" {
			value, err := strconv.ParseInt(strings.TrimSpace(parts[4]), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse resource index line %d packed size: %w", lineNumber, err)
			}
			entry.PackedSize = value
		}
		if entry.ResourcePath == "" || entry.HashPath == "" {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func DiscoverStaticClientResources(clientRoot string) ([]StaticResourceDiscovery, error) {
	entries, _, err := ReadResourceIndex(clientRoot)
	if err != nil {
		return nil, err
	}
	out := make([]StaticResourceDiscovery, 0)
	for _, entry := range entries {
		kind, priority, ok := classifyStaticResource(entry.ResourcePath)
		if !ok {
			continue
		}
		evidence, err := ResourceEvidence(clientRoot, entry)
		if err != nil {
			continue
		}
		out = append(out, StaticResourceDiscovery{
			Kind:         kind,
			Priority:     priority,
			ResourcePath: entry.ResourcePath,
			Evidence:     evidence,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].Evidence.ResourcePath < out[j].Evidence.ResourcePath
		}
		return out[i].Priority < out[j].Priority
	})
	return out, nil
}

func classifyStaticResource(resourcePath string) (string, int, bool) {
	value := strings.ToLower(strings.TrimSpace(resourcePath))
	if !strings.HasPrefix(value, "res:/staticdata/") {
		return "", 0, false
	}
	switch {
	case value == "res:/staticdata/types.fsdbinary":
		return "type_metadata", 10, true
	case strings.Contains(value, "blueprint"):
		return "blueprint_metadata", 20, true
	case strings.Contains(value, "recipe") || strings.Contains(value, "industry") || strings.Contains(value, "manufactur"):
		return "recipe_metadata", 30, true
	case strings.Contains(value, "typematerial") || strings.Contains(value, "materialrequirements"):
		return "material_requirement_metadata", 40, true
	default:
		return "", 0, false
	}
}

func ReadResourceIndex(clientRoot string) (ResourceIndex, string, error) {
	for _, name := range []string{"resfileindex.txt", "resfileindex_prefetch.txt", "resfileindex_Windows.txt"} {
		path := filepath.Join(clientRoot, name)
		file, err := os.Open(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, "", err
		}
		defer file.Close()
		entries, err := ParseResourceIndex(file)
		if err != nil {
			return nil, "", err
		}
		return entries, path, nil
	}
	return nil, "", fmt.Errorf("no resfileindex file found under %s", clientRoot)
}

func (idx ResourceIndex) Find(resourcePath string) (ResourceIndexEntry, bool) {
	resourcePath = strings.ToLower(strings.TrimSpace(resourcePath))
	for _, entry := range idx {
		if strings.ToLower(entry.ResourcePath) == resourcePath {
			return entry, true
		}
	}
	return ResourceIndexEntry{}, false
}

func ResolveResourcePath(clientRoot string, entry ResourceIndexEntry) (string, error) {
	if strings.TrimSpace(entry.HashPath) == "" {
		return "", errors.New("resource hash path is empty")
	}
	hashPathParts := strings.Split(filepath.ToSlash(entry.HashPath), "/")
	candidates := []string{
		filepath.Join(append([]string{clientRoot, "ResFiles"}, hashPathParts...)...),
		filepath.Join(append([]string{filepath.Dir(clientRoot), "ResFiles"}, hashPathParts...)...),
		filepath.Join(append([]string{clientRoot}, hashPathParts...)...),
		filepath.Join(append([]string{filepath.Dir(clientRoot)}, hashPathParts...)...),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("resource %s was not found near %s", entry.ResourcePath, clientRoot)
}

func ResourceEvidence(clientRoot string, entry ResourceIndexEntry) (StaticResourceEvidence, error) {
	path, err := ResolveResourcePath(clientRoot, entry)
	if err != nil {
		return StaticResourceEvidence{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return StaticResourceEvidence{}, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return StaticResourceEvidence{}, err
	}
	return StaticResourceEvidence{
		ResourcePath: entry.ResourcePath,
		HashPath:     entry.HashPath,
		Path:         path,
		SHA256:       hex.EncodeToString(hash.Sum(nil)),
		SizeBytes:    size,
		IndexSize:    entry.IndexSize,
		PackedSize:   entry.PackedSize,
	}, nil
}
