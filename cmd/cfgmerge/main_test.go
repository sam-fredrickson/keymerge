// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/goccy/go-yaml"
)

//go:embed testfiles
var testfiles embed.FS

// writeEmbeddedFile creates a temporary file with content from the embedded filesystem.
func writeEmbeddedFile(t *testing.T, tmpDir, embeddedPath string) string {
	t.Helper()
	content, err := fs.ReadFile(testfiles, embeddedPath)
	if err != nil {
		t.Fatalf("failed to read embedded file %s: %v", embeddedPath, err)
	}

	filename := filepath.Base(embeddedPath)
	tmpFile := filepath.Join(tmpDir, filename)
	if err := os.WriteFile(tmpFile, content, 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	return tmpFile
}

func TestRunMergeFormats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfgmerge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write all embedded files to temp directory
	baseYAML := writeEmbeddedFile(t, tmpDir, "testfiles/base.yaml")
	baseJSON := writeEmbeddedFile(t, tmpDir, "testfiles/base.json")
	baseTOML := writeEmbeddedFile(t, tmpDir, "testfiles/base.toml")

	overlayYAML := writeEmbeddedFile(t, tmpDir, "testfiles/overlay.yaml")
	overlayJSON := writeEmbeddedFile(t, tmpDir, "testfiles/overlay.json")
	overlayTOML := writeEmbeddedFile(t, tmpDir, "testfiles/overlay.toml")

	// Read expected result (from YAML merge, applicable to all YAML-based test cases)
	expectedContent, err := fs.ReadFile(testfiles, "testfiles/expected.json")
	if err != nil {
		t.Fatalf("failed to read expected.json: %v", err)
	}

	var expected map[string]any
	if err := json.Unmarshal(expectedContent, &expected); err != nil {
		t.Fatalf("failed to unmarshal expected.json: %v", err)
	}

	tests := []struct {
		name         string
		baseFile     string
		overlayFile  string
		outputFormat format
	}{
		// Same-format tests
		{"yaml to yaml", baseYAML, overlayYAML, "yaml"},
		{"yaml to json", baseYAML, overlayYAML, "json"},
		{"yaml to toml", baseYAML, overlayYAML, "toml"},
		{"json to yaml", baseJSON, overlayJSON, "yaml"},
		{"json to json", baseJSON, overlayJSON, "json"},
		{"json to toml", baseJSON, overlayJSON, "toml"},
		{"toml to yaml", baseTOML, overlayTOML, "yaml"},
		{"toml to json", baseTOML, overlayTOML, "json"},
		{"toml to toml", baseTOML, overlayTOML, "toml"},

		// Cross-format merge tests (mix different input formats)
		{"yaml base, json overlay to yaml", baseYAML, overlayJSON, "yaml"},
		{"json base, yaml overlay to json", baseJSON, overlayYAML, "json"},
		{"yaml base, toml overlay to toml", baseYAML, overlayTOML, "toml"},
		{"toml base, json overlay to json", baseTOML, overlayJSON, "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			err := Run(nil, 0, 0, "_delete", []string{tt.baseFile, tt.overlayFile}, tt.outputFormat, &output)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			var result map[string]any
			switch tt.outputFormat {
			case "json":
				if err := json.Unmarshal(output.Bytes(), &result); err != nil {
					t.Fatalf("failed to unmarshal result as JSON: %v", err)
				}
			case "yaml":
				if err := yaml.Unmarshal(output.Bytes(), &result); err != nil {
					t.Fatalf("failed to unmarshal result as YAML: %v", err)
				}
			case "toml":
				if err := toml.Unmarshal(output.Bytes(), &result); err != nil {
					t.Fatalf("failed to unmarshal result as TOML: %v", err)
				}
			}

			// Normalize types by marshaling to JSON and back so comparisons are consistent
			// across different format unmarshalers (YAML, JSON, TOML produce different types).
			resultJSON, err := json.Marshal(result)
			if err != nil {
				t.Fatalf("failed to marshal result: %v", err)
			}
			var normalized map[string]any
			if err := json.Unmarshal(resultJSON, &normalized); err != nil {
				t.Fatalf("failed to unmarshal normalized result: %v", err)
			}

			if !reflect.DeepEqual(normalized, expected) {
				t.Errorf("result does not match expected.\nGot: %#v\nExpected: %#v", normalized, expected)
			}
		})
	}
}

func TestRunMissingFiles(t *testing.T) {
	var output bytes.Buffer
	err := Run(nil, 0, 0, "_delete", []string{}, "", &output)
	if err == nil {
		t.Errorf("expected error for missing files, got nil")
	}
	if !strings.Contains(err.Error(), "no files") {
		t.Errorf("expected 'no files' error, got: %v", err)
	}
}

func TestRunFileNotFound(t *testing.T) {
	var output bytes.Buffer
	err := Run(nil, 0, 0, "_delete", []string{"nonexistent.yaml"}, "", &output)
	if err == nil {
		t.Errorf("expected error for missing file, got nil")
	}
}

func TestRunUnknownFormat(t *testing.T) {
	// Create a temporary directory and file with unknown extension
	tmpDir, err := os.MkdirTemp("", "cfgmerge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "test.unknown")
	if err := os.WriteFile(tmpFile, []byte("key: value"), 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	var output bytes.Buffer
	err = Run(nil, 0, 0, "_delete", []string{tmpFile}, "", &output)
	if err == nil {
		t.Errorf("expected error for unknown format, got nil")
	}
}

func TestPrimaryKeysFlag(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single key",
			input:    "name",
			expected: []string{"name"},
		},
		{
			name:     "multiple keys",
			input:    "name,id",
			expected: []string{"name", "id"},
		},
		{
			name:     "multiple calls",
			input:    "name",
			expected: []string{"name"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var pk primaryKeys
			if err := pk.Set(tt.input); err != nil {
				t.Fatalf("unexpected error from Set: %v", err)
			}
			keys := pk.Keys()
			if len(keys) != len(tt.expected) {
				t.Errorf("expected %d keys, got %d", len(tt.expected), len(keys))
			}
			for i, k := range keys {
				if k != tt.expected[i] {
					t.Errorf("expected key[%d] = %s, got %s", i, tt.expected[i], k)
				}
			}
		})
	}
}

func TestScalarModeFlag(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"concat", "concat", true},
		{"dedup", "dedup", true},
		{"replace", "replace", true},
		{"empty", "", true},
		{"invalid", "invalid_mode", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sm scalarMode
			err := sm.Set(tt.input)
			if (err == nil) != tt.valid {
				t.Errorf("expected valid=%v, got error=%v", tt.valid, err)
			}
		})
	}
}

func TestDupeModeFlag(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"unique", "unique", true},
		{"consolidate", "consolidate", true},
		{"empty", "", true},
		{"invalid", "invalid_mode", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dm dupeMode
			err := dm.Set(tt.input)
			if (err == nil) != tt.valid {
				t.Errorf("expected valid=%v, got error=%v", tt.valid, err)
			}
		})
	}
}

func TestFormatFlag(t *testing.T) {
	tests := []struct {
		name  string
		input string
		valid bool
	}{
		{"json", "json", true},
		{"yaml", "yaml", true},
		{"toml", "toml", true},
		{"empty", "", true},
		{"invalid", "xml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fmt format
			err := fmt.Set(tt.input)
			if (err == nil) != tt.valid {
				t.Errorf("expected valid=%v, got error=%v", tt.valid, err)
			}
		})
	}
}

func TestTOMLMarshalNonMapRoot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfgmerge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create two JSON files with top-level arrays
	// When merged, the result will be a top-level array, which TOML cannot represent
	baseFile := filepath.Join(tmpDir, "base.json")
	overlayFile := filepath.Join(tmpDir, "overlay.json")

	if err := os.WriteFile(baseFile, []byte(`[{"name":"a","value":1}]`), 0o600); err != nil {
		t.Fatalf("failed to write base.json: %v", err)
	}
	if err := os.WriteFile(overlayFile, []byte(`[{"name":"b","value":2}]`), 0o600); err != nil {
		t.Fatalf("failed to write overlay.json: %v", err)
	}

	var output bytes.Buffer
	err = Run(nil, 0, 0, "_delete", []string{baseFile, overlayFile}, "toml", &output)
	if err == nil {
		t.Errorf("expected error when marshaling top-level array as TOML, got nil")
	}
}
