package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Validator struct {
	baseDir string
	cache   map[string]*jsonschema.Schema
}

func NewValidator(baseDir string) *Validator {
	return &Validator{
		baseDir: baseDir,
		cache:   make(map[string]*jsonschema.Schema),
	}
}

func (v *Validator) ValidateFile(schemaName, inputPath string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	return v.ValidateBytes(schemaName, data)
}

func (v *Validator) ValidateBytes(schemaName string, data []byte) error {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	schema, err := v.schema(schemaName)
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("validate %s: %w", schemaName, err)
	}
	return nil
}

func (v *Validator) schema(schemaName string) (*jsonschema.Schema, error) {
	if schema, ok := v.cache[schemaName]; ok {
		return schema, nil
	}
	path := filepath.Join(v.baseDir, schemaName)
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(path)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", schemaName, err)
	}
	v.cache[schemaName] = schema
	return schema, nil
}
