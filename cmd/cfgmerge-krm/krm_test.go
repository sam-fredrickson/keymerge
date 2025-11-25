// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

//go:embed testfiles/basic-input.yaml
var basicInput []byte

//go:embed testfiles/basic-output.yaml
var basicOutput []byte

//go:embed testfiles/multi-group-input.yaml
var multiGroupInput []byte

//go:embed testfiles/multi-group-output.yaml
var multiGroupOutput []byte

func TestRun_EndToEnd(t *testing.T) {
	tests := []struct {
		name   string
		input  []byte
		output []byte
	}{
		{name: "basic merge", input: basicInput, output: basicOutput},
		{name: "multiple groups", input: multiGroupInput, output: multiGroupOutput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := Run(bytes.NewReader(tt.input), &output); err != nil {
				t.Fatalf("Run failed: %v", err)
			}
			compareResourceLists(t, tt.output, output.Bytes())
		})
	}
}

func TestRun_MergeOptions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		validate func(t *testing.T, config map[string]any)
	}{
		{
			name: "per-ConfigMap scalar mode",
			input: `
apiVersion: v1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: base
      annotations:
        config.keymerge.io/id: "test"
        config.keymerge.io/order: "0"
        config.keymerge.io/final-name: "final"
    data:
      config.yaml: |
        tags: [a, b]
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: overlay
      annotations:
        config.keymerge.io/id: "test"
        config.keymerge.io/order: "10"
        config.keymerge.io/scalar-mode: "dedup"
    data:
      config.yaml: |
        tags: [b, c]
`,
			validate: func(t *testing.T, config map[string]any) {
				tags, ok := config["tags"].([]any)
				if !ok {
					t.Fatal("tags is not an array")
				}
				// With dedup mode, [a,b] + [b,c] should be [a,b,c] (deduplicated)
				expected := []string{"a", "b", "c"}
				if len(tags) != len(expected) {
					t.Fatalf("Expected tags %v, got %v", expected, tags)
				}
			},
		},
		{
			name: "custom primary keys with whitespace",
			input: `
apiVersion: v1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: base
      annotations:
        config.keymerge.io/id: "test"
        config.keymerge.io/order: "0"
        config.keymerge.io/final-name: "final"
        config.keymerge.io/keys: " id , uuid , name "
    data:
      config.yaml: |
        items:
          - id: 1
            value: base
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: overlay
      annotations:
        config.keymerge.io/id: "test"
        config.keymerge.io/order: "10"
    data:
      config.yaml: |
        items:
          - id: 1
            value: overlay
`,
			validate: func(t *testing.T, config map[string]any) {
				items, ok := config["items"].([]any)
				if !ok || len(items) != 1 {
					t.Fatalf("Expected 1 item after merge, got: %v", config)
				}
				item := items[0].(map[string]any)
				if item["value"] != "overlay" {
					t.Errorf("Expected value='overlay', got: %v", item["value"])
				}
			},
		},
		{
			// This test verifies that when a middle ConfigMap doesn't have the data key,
			// the options from later ConfigMaps are still correctly applied.
			// Bug scenario: CM0 has config.yaml, CM1 doesn't, CM2 has config.yaml with scalar-mode=replace
			// Without fix: CM2's content would use CM1's options (wrong!)
			name: "options aligned when middle ConfigMap missing data key",
			input: `
apiVersion: v1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: base
      annotations:
        config.keymerge.io/id: "test"
        config.keymerge.io/order: "0"
        config.keymerge.io/final-name: "final"
    data:
      config.yaml: |
        tags: [a, b]
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: middle-no-data
      annotations:
        config.keymerge.io/id: "test"
        config.keymerge.io/order: "5"
        config.keymerge.io/scalar-mode: "concat"
    data:
      other.yaml: |
        unrelated: data
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: overlay-with-replace
      annotations:
        config.keymerge.io/id: "test"
        config.keymerge.io/order: "10"
        config.keymerge.io/scalar-mode: "replace"
    data:
      config.yaml: |
        tags: [x, y]
`,
			validate: func(t *testing.T, config map[string]any) {
				tags, ok := config["tags"].([]any)
				if !ok {
					t.Fatal("tags is not an array")
				}
				// With replace mode from CM2, [a,b] should be replaced by [x,y]
				// If bug exists, concat mode from CM1 would be used: [a,b,x,y]
				expected := []string{"x", "y"}
				if len(tags) != len(expected) {
					t.Fatalf("Expected tags %v (replace mode), got %v (wrong options used?)", expected, tags)
				}
				for i, exp := range expected {
					if tags[i] != exp {
						t.Errorf("tags[%d]: expected %q, got %v", i, exp, tags[i])
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runAndValidate(t, tt.input, "config.yaml", tt.validate)
		})
	}
}

func TestRun_FormatDetection(t *testing.T) {
	tests := []struct {
		name        string
		ext         string
		baseData    string
		overlayData string
		validate    func(t *testing.T, config map[string]any)
	}{
		{
			name:        "JSON",
			ext:         "json",
			baseData:    `{"foo": 1}`,
			overlayData: `{"bar": 2}`,
			validate: func(t *testing.T, config map[string]any) {
				validateMergedKeys(t, config, "foo", "bar")
				if fooVal, ok := config["foo"].(float64); !ok || fooVal != 1.0 {
					t.Errorf("Expected foo=1.0, got %v", config["foo"])
				}
				if barVal, ok := config["bar"].(float64); !ok || barVal != 2.0 {
					t.Errorf("Expected bar=2.0, got %v", config["bar"])
				}
			},
		},
		{
			name:        "YML",
			ext:         "yml",
			baseData:    "foo: 1",
			overlayData: "bar: 2",
			validate: func(t *testing.T, config map[string]any) {
				validateMergedKeys(t, config, "foo", "bar")
			},
		},
		{
			name:        "TOML",
			ext:         "toml",
			baseData:    "foo = 1\n[database]\nhost = \"localhost\"",
			overlayData: "bar = 2\n[database]\nport = 5432",
			validate: func(t *testing.T, config map[string]any) {
				validateMergedKeys(t, config, "foo", "bar", "database")
				validateNestedKey(t, config, "database", "host")
				validateNestedKey(t, config, "database", "port")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := buildTwoConfigMapInput(tt.ext, tt.baseData, tt.overlayData)
			cm := runAndExtractFirst(t, input)
			config := parseConfigData(t, cm, "config."+tt.ext)
			tt.validate(t, config)
		})
	}
}

func TestRun_AnnotationFiltering(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		targetConfigMap   string
		wantPreserved     map[string]string
		wantRemovedPrefix string
	}{
		{
			name:              "removes keymerge annotations",
			input:             string(basicInput),
			targetConfigMap:   "final-app-config",
			wantRemovedPrefix: "config.keymerge.io/",
		},
		{
			name: "preserves non-keymerge annotations",
			input: `apiVersion: v1
kind: ResourceList
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: base
      annotations:
        config.keymerge.io/id: "test"
        config.keymerge.io/order: "0"
        config.keymerge.io/final-name: "final"
        custom.example.com/annotation: "should-be-preserved"
        another-annotation: "also-preserved"
    data:
      config.yaml: |
        foo: bar`,
			targetConfigMap: "final",
			wantPreserved: map[string]string{
				"custom.example.com/annotation": "should-be-preserved",
				"another-annotation":            "also-preserved",
			},
			wantRemovedPrefix: "config.keymerge.io/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := Run(strings.NewReader(tt.input), &output); err != nil {
				t.Fatalf("Run failed: %v", err)
			}

			var result ResourceList
			if err := yaml.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("Failed to unmarshal output: %v", err)
			}

			// Find the target ConfigMap
			cm := findConfigMapByName(t, result.Items, tt.targetConfigMap)

			// Check preserved annotations
			for key, want := range tt.wantPreserved {
				if got := cm.Annotations[key]; got != want {
					t.Errorf("Annotation %q = %q, want %q", key, got, want)
				}
			}

			// Check removed annotations
			if tt.wantRemovedPrefix != "" {
				for key := range cm.Annotations {
					if strings.HasPrefix(key, tt.wantRemovedPrefix) {
						t.Errorf("Found annotation with prefix %q: %s", tt.wantRemovedPrefix, key)
					}
				}
			}
		})
	}
}

// Error cases

func TestRun_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantError   string
	}{
		{
			name:        "missing order",
			annotations: map[string]string{"config.keymerge.io/order": ""},
			wantError:   "order",
		},
		{
			name:        "missing final-name",
			annotations: map[string]string{"config.keymerge.io/final-name": ""},
			wantError:   "final-name",
		},
		{
			name:        "invalid order",
			annotations: map[string]string{"config.keymerge.io/order": "not-a-number"},
			wantError:   "invalid",
		},
		{
			name:        "no base ConfigMap",
			annotations: map[string]string{"config.keymerge.io/order": "10"},
			wantError:   "order=0",
		},
		{
			name:        "invalid scalar mode",
			annotations: map[string]string{"config.keymerge.io/scalar-mode": "invalid-mode"},
			wantError:   "scalar",
		},
		{
			name:        "invalid dupe mode",
			annotations: map[string]string{"config.keymerge.io/dupe-mode": "invalid-mode"},
			wantError:   "dupe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := buildErrorTestInput(tt.annotations)
			expectError(t, input, tt.wantError)
		})
	}
}

func TestRun_ValidModes(t *testing.T) {
	tests := []struct {
		annotation string
		value      string
	}{
		// dupe-mode variations
		{"dupe-mode", "unique"},
		{"dupe-mode", "consolidate"},
		{"dupe-mode", "UNIQUE"},
		{"dupe-mode", "CONSOLIDATE"},
		{"dupe-mode", " unique "},
		{"dupe-mode", " consolidate "},
		// scalar-mode variations
		{"scalar-mode", "concat"},
		{"scalar-mode", "dedup"},
		{"scalar-mode", "replace"},
		{"scalar-mode", "CONCAT"},
		{"scalar-mode", "DEDUP"},
		{"scalar-mode", "REPLACE"},
		{"scalar-mode", " concat "},
		{"scalar-mode", " dedup "},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("%s=%s", tt.annotation, tt.value)
		t.Run(name, func(t *testing.T) {
			input := buildMinimalInput(map[string]string{
				"config.keymerge.io/" + tt.annotation: tt.value,
			})

			var output bytes.Buffer
			if err := Run(strings.NewReader(input), &output); err != nil {
				t.Fatalf("Valid %s %q should not error: %v", tt.annotation, tt.value, err)
			}
		})
	}
}

// Helper functions

// configMapBuilder builds ConfigMap specs for ResourceList creation.
type configMapBuilder struct {
	name        string
	annotations map[string]string
	data        map[string]string
}

// newConfigMap creates a new ConfigMap builder with the given name.
func newConfigMap(name string) *configMapBuilder {
	return &configMapBuilder{
		name:        name,
		annotations: make(map[string]string),
		data:        make(map[string]string),
	}
}

// withAnnotation adds an annotation to the ConfigMap.
func (b *configMapBuilder) withAnnotation(key, value string) *configMapBuilder {
	b.annotations[key] = value
	return b
}

// withData adds a data key-value pair to the ConfigMap.
func (b *configMapBuilder) withData(key, value string) *configMapBuilder {
	b.data[key] = value
	return b
}

// buildResourceList creates a ResourceList YAML with the specified ConfigMaps.
func buildResourceList(configMaps ...*configMapBuilder) string {
	var items strings.Builder

	for _, cm := range configMaps {
		items.WriteString("  - apiVersion: v1\n")
		items.WriteString("    kind: ConfigMap\n")
		items.WriteString("    metadata:\n")
		items.WriteString(fmt.Sprintf("      name: %s\n", cm.name))

		if len(cm.annotations) > 0 {
			items.WriteString("      annotations:\n")
			for k, v := range cm.annotations {
				items.WriteString(fmt.Sprintf("        %s: %q\n", k, v))
			}
		}

		if len(cm.data) > 0 {
			items.WriteString("    data:\n")
			for k, v := range cm.data {
				items.WriteString(fmt.Sprintf("      %s: |\n", k))
				// Indent each line of data
				for _, line := range strings.Split(v, "\n") {
					if line != "" {
						items.WriteString("        " + line + "\n")
					}
				}
			}
		}
	}

	return fmt.Sprintf(`apiVersion: v1
kind: ResourceList
items:
%s`, items.String())
}

// buildMinimalInput creates a minimal ResourceList with one ConfigMap.
func buildMinimalInput(annotations map[string]string) string {
	cm := newConfigMap("base").
		withAnnotation("config.keymerge.io/id", "test").
		withAnnotation("config.keymerge.io/order", "0").
		withAnnotation("config.keymerge.io/final-name", "final").
		withData("config.yaml", "foo: bar")

	for k, v := range annotations {
		cm.withAnnotation(k, v)
	}

	return buildResourceList(cm)
}

// buildErrorTestInput creates a ResourceList with specified annotations, omitting any with empty values.
func buildErrorTestInput(annotations map[string]string) string {
	cm := newConfigMap("base").
		withData("config.yaml", "foo: bar")

	// Start with defaults
	defaults := map[string]string{
		"config.keymerge.io/id":         "test",
		"config.keymerge.io/order":      "0",
		"config.keymerge.io/final-name": "final",
	}

	// Apply defaults first, then overrides
	for k, v := range defaults {
		if overrideVal, hasOverride := annotations[k]; hasOverride {
			if overrideVal != "" {
				cm.withAnnotation(k, overrideVal)
			}
			// Empty value means omit the annotation
		} else {
			cm.withAnnotation(k, v)
		}
	}

	// Add any additional annotations not in defaults
	for k, v := range annotations {
		if _, isDefault := defaults[k]; !isDefault && v != "" {
			cm.withAnnotation(k, v)
		}
	}

	return buildResourceList(cm)
}

// buildTwoConfigMapInput creates a ResourceList with base and overlay ConfigMaps.
func buildTwoConfigMapInput(ext, baseData, overlayData string) string {
	base := newConfigMap("base").
		withAnnotation("config.keymerge.io/id", "test").
		withAnnotation("config.keymerge.io/order", "0").
		withAnnotation("config.keymerge.io/final-name", "final").
		withData("config."+ext, baseData)

	overlay := newConfigMap("overlay").
		withAnnotation("config.keymerge.io/id", "test").
		withAnnotation("config.keymerge.io/order", "10").
		withData("config."+ext, overlayData)

	return buildResourceList(base, overlay)
}

// runAndExtractFirst runs the KRM function and extracts the first ConfigMap.
func runAndExtractFirst(t *testing.T, input string) ConfigMap {
	t.Helper()

	var output bytes.Buffer
	if err := Run(strings.NewReader(input), &output); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	var result ResourceList
	if err := yaml.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("Failed to unmarshal output: %v", err)
	}

	if len(result.Items) == 0 {
		t.Fatal("No items in output")
	}

	cm, err := extractConfigMap(result.Items[0])
	if err != nil {
		t.Fatalf("Failed to extract ConfigMap: %v", err)
	}

	return cm
}

// parseConfigData parses a ConfigMap's data key as YAML.
func parseConfigData(t *testing.T, cm ConfigMap, key string) map[string]any {
	t.Helper()

	var config map[string]any
	if err := yaml.Unmarshal([]byte(cm.Data[key]), &config); err != nil {
		t.Fatalf("Failed to unmarshal %s: %v", key, err)
	}

	return config
}

// runAndValidate runs the KRM function and calls the validator on the merged config data.
func runAndValidate(t *testing.T, input string, dataKey string, validate func(t *testing.T, config map[string]any)) {
	t.Helper()

	cm := runAndExtractFirst(t, input)
	config := parseConfigData(t, cm, dataKey)
	validate(t, config)
}

// validateMergedKeys checks that all expected keys exist in the merged config.
func validateMergedKeys(t *testing.T, config map[string]any, keys ...string) {
	t.Helper()
	for _, key := range keys {
		if _, ok := config[key]; !ok {
			t.Errorf("Expected key %q in merged config", key)
		}
	}
}

// validateNestedKey checks that a nested key path exists in the merged config.
func validateNestedKey(t *testing.T, config map[string]any, path ...string) {
	t.Helper()
	current := config
	for i, key := range path {
		val, ok := current[key]
		if !ok {
			t.Errorf("Expected key %q in path %v", key, path[:i+1])
			return
		}
		if i < len(path)-1 {
			current, ok = val.(map[string]any)
			if !ok {
				t.Errorf("Expected %q to be a map at path %v", key, path[:i+1])
				return
			}
		}
	}
}

// expectError runs the KRM function and expects an error containing the specified substring.
func expectError(t *testing.T, input, wantError string) {
	t.Helper()
	var output bytes.Buffer
	err := Run(strings.NewReader(input), &output)
	if err == nil {
		t.Fatalf("Expected error containing %q, but got no error", wantError)
	}
	if !strings.Contains(err.Error(), wantError) {
		t.Fatalf("Expected error containing %q, got: %v", wantError, err)
	}
}

func compareResourceLists(t *testing.T, expected, actual []byte) {
	t.Helper()

	var expectedRL, actualRL ResourceList
	if err := yaml.Unmarshal(expected, &expectedRL); err != nil {
		t.Fatalf("Failed to unmarshal expected ResourceList: %v", err)
	}
	if err := yaml.Unmarshal(actual, &actualRL); err != nil {
		t.Fatalf("Failed to unmarshal actual ResourceList: %v", err)
	}

	// Basic metadata checks
	if expectedRL.APIVersion != actualRL.APIVersion {
		t.Errorf("APIVersion mismatch: expected %s, got %s", expectedRL.APIVersion, actualRL.APIVersion)
	}
	if expectedRL.Kind != actualRL.Kind {
		t.Errorf("Kind mismatch: expected %s, got %s", expectedRL.Kind, actualRL.Kind)
	}
	if len(expectedRL.Items) != len(actualRL.Items) {
		t.Fatalf("Item count mismatch: expected %d, got %d", len(expectedRL.Items), len(actualRL.Items))
	}

	// Index and compare items by name
	expectedItems := indexItemsByName(expectedRL.Items)
	actualItems := indexItemsByName(actualRL.Items)

	for name, expectedItem := range expectedItems {
		actualItem, ok := actualItems[name]
		if !ok {
			t.Errorf("Expected item %q not found in actual output", name)
			continue
		}

		// Use yaml.Marshal for canonical comparison (handles formatting differences)
		expectedYAML, _ := yaml.Marshal(expectedItem)
		actualYAML, _ := yaml.Marshal(actualItem)

		if string(expectedYAML) != string(actualYAML) {
			t.Errorf("Item %q mismatch:\nExpected:\n%s\nActual:\n%s",
				name, expectedYAML, actualYAML)
		}
	}
}

func indexItemsByName(items []map[string]any) map[string]map[string]any {
	index := make(map[string]map[string]any)
	for _, item := range items {
		if metadata, ok := item["metadata"].(map[string]any); ok {
			if name, ok := metadata["name"].(string); ok {
				index[name] = item
			}
		}
	}
	return index
}

func extractConfigMap(item map[string]any) (ConfigMap, error) {
	data, err := yaml.Marshal(item)
	if err != nil {
		return ConfigMap{}, err
	}

	var cm ConfigMap
	if err := yaml.Unmarshal(data, &cm); err != nil {
		return ConfigMap{}, err
	}

	return cm, nil
}

// findConfigMapByName finds a ConfigMap by name in the ResourceList items.
func findConfigMapByName(t *testing.T, items []map[string]any, name string) ConfigMap {
	t.Helper()

	for _, item := range items {
		kind, _ := item["kind"].(string)
		if kind != "ConfigMap" {
			continue
		}

		metadata, _ := item["metadata"].(map[string]any)
		cmName, _ := metadata["name"].(string)
		if cmName != name {
			continue
		}

		cm, err := extractConfigMap(item)
		if err != nil {
			t.Fatalf("Failed to extract ConfigMap %q: %v", name, err)
		}
		return cm
	}

	t.Fatalf("ConfigMap %q not found in output", name)
	return ConfigMap{} // unreachable
}
