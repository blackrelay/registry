package sui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/blackrelay/registry/internal/model"
)

type PackageManifest struct {
	Name               string               `json:"name"`
	Network            string               `json:"network"`
	OriginalPackageID  string               `json:"originalPackageId"`
	PublishedPackageID string               `json:"publishedPackageId"`
	StartingCheckpoint uint64               `json:"startingCheckpoint,omitempty"`
	Cycle              *int                 `json:"cycle,omitempty"`
	Modules            []string             `json:"modules"`
	ObjectTypes        []ObjectTypeManifest `json:"objectTypes,omitempty"`
}

type ObjectTypeManifest struct {
	ModuleName string `json:"module"`
	TypeName   string `json:"type"`
}

type Manifest struct {
	SchemaVersion string            `json:"schemaVersion"`
	Packages      []PackageManifest `json:"packages"`
}

type BackfillPlan struct {
	Environment model.Environment `json:"environment"`
	Manifest    Manifest          `json:"manifest"`
	MaxPages    int               `json:"maxPages"`
	Concurrency int               `json:"concurrency"`
}

func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (m Manifest) Validate() error {
	if m.SchemaVersion != "registry.sui-packages.v1" {
		return fmt.Errorf("unsupported Sui package manifest version %q", m.SchemaVersion)
	}
	if len(m.Packages) == 0 {
		return errors.New("sui package manifest must contain at least one package")
	}
	for _, pkg := range m.Packages {
		if strings.TrimSpace(pkg.Name) == "" || strings.TrimSpace(pkg.Network) == "" {
			return errors.New("sui package manifest package name and network are required")
		}
		if !isSuiID(pkg.OriginalPackageID) || !isSuiID(pkg.PublishedPackageID) {
			return fmt.Errorf("sui package %s has an invalid package id", pkg.Name)
		}
		if len(pkg.Modules) == 0 {
			return fmt.Errorf("sui package %s has no modules", pkg.Name)
		}
		for _, objectType := range pkg.ObjectTypes {
			if !isMoveIdentifier(objectType.ModuleName) || !isMoveIdentifier(objectType.TypeName) {
				return fmt.Errorf("sui package %s has invalid object type %s::%s", pkg.Name, objectType.ModuleName, objectType.TypeName)
			}
		}
	}
	return nil
}

func isSuiID(value string) bool {
	if !strings.HasPrefix(value, "0x") || len(value) < 3 {
		return false
	}
	for _, r := range value[2:] {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F') {
			continue
		}
		return false
	}
	return true
}
