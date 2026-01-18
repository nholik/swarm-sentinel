package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMappingFile_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "stacks.yaml")

	yaml := `stacks:
  - name: prod
    compose_url: https://example.com/prod/compose.yml
  - name: staging
    compose_url: https://example.com/staging/compose.yml
    timeout: 20s
  - name: monitoring
    compose_url: https://example.com/monitoring/compose.yml
`

	if err := os.WriteFile(yamlFile, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	mappings, err := LoadMappingFile(yamlFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mappings) != 3 {
		t.Fatalf("expected 3 mappings, got %d", len(mappings))
	}

	if mappings[0].Name != "prod" || mappings[0].ComposeURL != "https://example.com/prod/compose.yml" {
		t.Fatalf("unexpected prod mapping: %+v", mappings[0])
	}

	if mappings[1].Timeout != 20*time.Second {
		t.Fatalf("unexpected staging timeout: %s", mappings[1].Timeout)
	}
}

func TestLoadMappingFile_EmptyPath(t *testing.T) {
	mappings, err := LoadMappingFile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mappings != nil {
		t.Fatalf("expected nil for empty path, got %+v", mappings)
	}
}

func TestLoadMappingFile_FileNotFound(t *testing.T) {
	_, err := LoadMappingFile("/nonexistent/path/stacks.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadMappingFile_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "bad.yaml")

	if err := os.WriteFile(yamlFile, []byte("stacks: ["), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	_, err := LoadMappingFile(yamlFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadMappingFile_EmptyStacks(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "empty.yaml")

	if err := os.WriteFile(yamlFile, []byte("stacks: []"), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	_, err := LoadMappingFile(yamlFile)
	if err == nil || err.Error() != "mapping file contains no stacks" {
		t.Fatalf("expected 'no stacks' error, got %v", err)
	}
}

func TestLoadMappingFile_MissingName(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "no_name.yaml")

	yaml := `stacks:
  - compose_url: https://example.com/compose.yml
`

	if err := os.WriteFile(yamlFile, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	_, err := LoadMappingFile(yamlFile)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoadMappingFile_MissingComposeURL(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "no_url.yaml")

	yaml := `stacks:
  - name: prod
`

	if err := os.WriteFile(yamlFile, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	_, err := LoadMappingFile(yamlFile)
	if err == nil {
		t.Fatal("expected error for missing compose_url")
	}
}

func TestLoadMappingFile_InvalidURL(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "bad_url.yaml")

	yaml := `stacks:
  - name: prod
    compose_url: "ht!tp://invalid"
`

	if err := os.WriteFile(yamlFile, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	_, err := LoadMappingFile(yamlFile)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestLoadMappingFile_DuplicateNames(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "dups.yaml")

	yaml := `stacks:
  - name: prod
    compose_url: https://example.com/prod1.yml
  - name: prod
    compose_url: https://example.com/prod2.yml
`

	if err := os.WriteFile(yamlFile, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	_, err := LoadMappingFile(yamlFile)
	if err == nil || err.Error() != "stack \"prod\": duplicate name" {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestLoadMappingFile_NegativeTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	yamlFile := filepath.Join(tmpDir, "bad_timeout.yaml")

	yaml := `stacks:
  - name: prod
    compose_url: https://example.com/compose.yml
    timeout: -5s
`

	if err := os.WriteFile(yamlFile, []byte(yaml), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}

	_, err := LoadMappingFile(yamlFile)
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}
}
