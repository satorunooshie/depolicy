package depolicy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfigRejectsDuplicateKeys(t *testing.T) {
	_, err := ParseConfig([]byte("version: 1\nversion: 1\npolicies: []\n"), "test.yaml")
	if err == nil || !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("err = %v, want duplicate key error", err)
	}
}

func TestParseConfigRejectsAlias(t *testing.T) {
	input := []byte("version: 1\npackage-sets:\n  a: &a\n    - std:...\n  b: *a\npolicies: []\n")
	_, err := ParseConfig(input, "test.yaml")
	if err == nil || (!strings.Contains(err.Error(), "aliases") && !strings.Contains(err.Error(), "anchors")) {
		t.Fatalf("err = %v, want alias or anchor error", err)
	}
}

func TestParseConfigRejectsUnknownFields(t *testing.T) {
	_, err := ParseConfig([]byte("version: 1\nunknown: true\npolicies: []\n"), "test.yaml")
	if err == nil || !strings.Contains(err.Error(), "unknown top-level field") {
		t.Fatalf("err = %v, want unknown field error", err)
	}
}

func TestLoadProjectConfigWrapsCompileErrorsAsConfigErrors(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/example/project\n\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := []byte(`
version: 1
policies:
  - id: api
    packages:
      - local:api/...
    imports:
      default: allow
      rules:
        - id: deny-generated
          deny:
            - set:missing
`)
	configPath := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProjectConfig(configPath)
	var configErr *ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("err = %T %[1]v, want ConfigError", err)
	}
}
