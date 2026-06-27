#!/usr/bin/env sh
set -eu

root="published-exports"
prefixes="registry/current,registry/archive/all"

usage() {
  cat <<'EOF'
Usage: ./scripts/verify-publication.sh [options]

Options:
  --root PATH          Published object-store-shaped root, default published-exports
  --prefix LIST        Comma-separated prefixes, default registry/current,registry/archive/all
  -h, --help           Show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --root)
      root="${2:?--root requires a value}"
      shift 2
      ;;
    --prefix)
      prefixes="${2:?--prefix requires a value}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT INT TERM

checker="$tmp_dir/verify-publication.go"
cat > "$checker" <<'EOF'
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type pointerFile struct {
	SchemaVersion  string `json:"schemaVersion"`
	BundleID       string `json:"bundleId"`
	ManifestKey    string `json:"manifestKey"`
	ManifestSHA256 string `json:"manifestSha256"`
	Files          []struct {
		Path      string `json:"path"`
		ObjectKey string `json:"objectKey"`
		SHA256    string `json:"sha256"`
		SizeBytes *int64 `json:"sizeBytes"`
	} `json:"files"`
}

type manifestFile struct {
	SchemaVersion string `json:"schemaVersion"`
}

type prefixResult struct {
	Prefix           string   `json:"prefix"`
	NormalisedPrefix string   `json:"normalisedPrefix"`
	Valid            bool     `json:"valid"`
	Errors           []string `json:"errors"`
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: verify-publication <root> <comma-separated-prefixes>")
		os.Exit(2)
	}

	root := os.Args[1]
	rawPrefixes := strings.Split(os.Args[2], ",")
	results := make([]prefixResult, 0, len(rawPrefixes))
	allValid := true

	for _, rawPrefix := range rawPrefixes {
		rawPrefix = strings.TrimSpace(rawPrefix)
		result := verifyPrefix(root, rawPrefix)
		if !result.Valid {
			allValid = false
		}
		results = append(results, result)
	}

	out := map[string]any{
		"schemaVersion": "registry.publication_proof.v1",
		"verifiedAt":    time.Now().UTC().Format(time.RFC3339),
		"root":          root,
		"valid":         allValid,
		"prefixes":      results,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !allValid {
		os.Exit(1)
	}
}

func verifyPrefix(root, rawPrefix string) prefixResult {
	errorsList := []string{}
	prefix, err := normaliseObjectKey(rawPrefix)
	if err != nil {
		return prefixResult{Prefix: rawPrefix, Valid: false, Errors: []string{err.Error()}}
	}

	pointerKey := prefix + "/latest/manifest.json"
	pointerPath, err := resolveObjectPath(root, pointerKey)
	if err != nil {
		return prefixResult{Prefix: rawPrefix, NormalisedPrefix: prefix, Valid: false, Errors: []string{err.Error()}}
	}

	var pointer pointerFile
	if err := readJSON(pointerPath, &pointer); err != nil {
		errorsList = append(errorsList, "missing or invalid latest pointer "+pointerKey+": "+err.Error())
		return prefixResult{Prefix: rawPrefix, NormalisedPrefix: prefix, Valid: false, Errors: errorsList}
	}

	if pointer.SchemaVersion != "registry.export_publish_pointer.v1" {
		errorsList = append(errorsList, "unexpected pointer schemaVersion "+pointer.SchemaVersion)
	}
	if pointer.BundleID == "" {
		errorsList = append(errorsList, "pointer missing bundleId")
	}
	if pointer.ManifestKey == "" {
		errorsList = append(errorsList, "pointer missing manifestKey")
	}
	if pointer.ManifestSHA256 == "" {
		errorsList = append(errorsList, "pointer missing manifestSha256")
	}

	actualManifestKey, err := normaliseObjectKey(pointer.ManifestKey)
	if err != nil {
		errorsList = append(errorsList, "unsafe manifestKey: "+err.Error())
	} else {
		expectedManifestKey := prefix + "/bundles/" + pointer.BundleID + "/manifest.json"
		if pointer.BundleID != "" && actualManifestKey != expectedManifestKey {
			errorsList = append(errorsList, "manifestKey "+actualManifestKey+" did not match expected "+expectedManifestKey)
		}
		manifestPath, err := resolveObjectPath(root, actualManifestKey)
		if err != nil {
			errorsList = append(errorsList, err.Error())
		} else {
			hash, err := sha256File(manifestPath)
			if err != nil {
				errorsList = append(errorsList, "missing bundle manifest "+actualManifestKey+": "+err.Error())
			} else if pointer.ManifestSHA256 != "" && hash != pointer.ManifestSHA256 {
				errorsList = append(errorsList, "bundle manifest sha256 mismatch")
			}
			var manifest manifestFile
			if err := readJSON(manifestPath, &manifest); err != nil {
				errorsList = append(errorsList, "invalid bundle manifest "+actualManifestKey+": "+err.Error())
			} else if manifest.SchemaVersion != "registry.export_manifest.v1" {
				errorsList = append(errorsList, "unexpected bundle manifest schemaVersion "+manifest.SchemaVersion)
			}
		}
	}

	if len(pointer.Files) == 0 {
		errorsList = append(errorsList, "pointer listed no files")
	}
	for _, file := range pointer.Files {
		if file.ObjectKey == "" {
			errorsList = append(errorsList, "published file "+file.Path+" missing objectKey")
			continue
		}
		objectKey, err := normaliseObjectKey(file.ObjectKey)
		if err != nil {
			errorsList = append(errorsList, "unsafe object key "+file.ObjectKey+": "+err.Error())
			continue
		}
		objectPath, err := resolveObjectPath(root, objectKey)
		if err != nil {
			errorsList = append(errorsList, err.Error())
			continue
		}
		hash, err := sha256File(objectPath)
		if err != nil {
			errorsList = append(errorsList, "missing published object "+objectKey+": "+err.Error())
			continue
		}
		if file.SHA256 != "" && hash != file.SHA256 {
			errorsList = append(errorsList, "sha256 mismatch for "+objectKey)
		}
		if file.SizeBytes != nil {
			info, err := os.Stat(objectPath)
			if err != nil {
				errorsList = append(errorsList, "stat failed for "+objectKey+": "+err.Error())
			} else if info.Size() != *file.SizeBytes {
				errorsList = append(errorsList, "size mismatch for "+objectKey)
			}
		}
	}

	return prefixResult{Prefix: rawPrefix, NormalisedPrefix: prefix, Valid: len(errorsList) == 0, Errors: errorsList}
}

func normaliseObjectKey(key string) (string, error) {
	key = strings.Trim(strings.ReplaceAll(key, "\\", "/"), "/")
	if key == "" {
		return "", errors.New("object key is required")
	}
	parts := strings.Split(key, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("unsafe object key %q", key)
		}
	}
	return strings.Join(parts, "/"), nil
}

func resolveObjectPath(root, key string) (string, error) {
	key, err := normaliseObjectKey(key)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(key) {
		return "", fmt.Errorf("unsafe object key %q: absolute paths are not allowed", key)
	}
	path := root
	for _, part := range strings.Split(key, "/") {
		path = filepath.Join(path, part)
	}
	return path, nil
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
EOF

go run "$checker" "$root" "$prefixes"
