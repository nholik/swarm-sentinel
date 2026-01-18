package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// StackMapping represents a single stack → compose URL mapping.
type StackMapping struct {
	Name       string        `yaml:"name"`
	ComposeURL string        `yaml:"compose_url"`
	Timeout    time.Duration `yaml:"timeout,omitempty"`
}

// MappingFile is the parsed YAML structure for multi-stack configuration:
// stacks: [{name, compose_url, timeout}]
type MappingFile struct {
	Stacks []StackMapping `yaml:"stacks"`
}

// LoadMappingFile parses a YAML mapping file from the given path.
// Returns nil if path is empty (no mapping file).
func LoadMappingFile(path string) ([]StackMapping, error) {
	if path == "" {
		return nil, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mapping file: %w", err)
	}

	var mf MappingFile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parse mapping file: %w", err)
	}

	if err := validateMappings(mf.Stacks); err != nil {
		return nil, err
	}

	return mf.Stacks, nil
}

// validateMappings ensures all mappings are valid.
func validateMappings(mappings []StackMapping) error {
	if len(mappings) == 0 {
		return fmt.Errorf("mapping file contains no stacks")
	}

	seen := make(map[string]bool)

	for i, m := range mappings {
		if m.Name == "" {
			return fmt.Errorf("stack %d: name is required", i)
		}

		if m.ComposeURL == "" {
			return fmt.Errorf("stack %q: compose_url is required", m.Name)
		}

		if err := validateHTTPURL(m.ComposeURL, "compose_url"); err != nil {
			return fmt.Errorf("stack %q: %w", m.Name, err)
		}

		if seen[m.Name] {
			return fmt.Errorf("stack %q: duplicate name", m.Name)
		}
		seen[m.Name] = true

		if m.Timeout < 0 {
			return fmt.Errorf("stack %q: timeout cannot be negative", m.Name)
		}
	}

	return nil
}
