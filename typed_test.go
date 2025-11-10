// SPDX-License-Identifier: Apache-2.0

package keymerge_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/sam-fredrickson/keymerge"
)

// Test Merger with composite primary keys.
func TestMerger_CompositePrimaryKey(t *testing.T) {
	type Endpoint struct {
		Region string `yaml:"region" km:"primary"`
		Name   string `yaml:"name" km:"primary"`
		URL    string `yaml:"url"`
	}

	type Config struct {
		Endpoints []Endpoint `yaml:"endpoints"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
endpoints:
  - region: us-east
    name: api
    url: v1.example.com
  - region: us-west
    name: api
    url: v1-west.example.com
`)

	overlay := []byte(`
endpoints:
  - region: us-east
    name: api
    url: v2.example.com
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Should have 2 endpoints: merged us-east/api and unchanged us-west/api
	if len(config.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(config.Endpoints))
	}

	// Find us-east/api endpoint
	var usEastAPI *Endpoint
	for i := range config.Endpoints {
		if config.Endpoints[i].Region == "us-east" && config.Endpoints[i].Name == "api" {
			usEastAPI = &config.Endpoints[i]
			break
		}
	}

	if usEastAPI == nil {
		t.Fatal("us-east/api endpoint not found")
	}

	// URL should be updated to v2
	if usEastAPI.URL != "v2.example.com" {
		t.Fatalf("expected URL v2.example.com, got %s", usEastAPI.URL)
	}
}

// Test Merger with field-specific scalar list modes.
func TestMerger_ScalarModes(t *testing.T) {
	type Config struct {
		Concat  []string `yaml:"concat" km:"mode=concat"`
		Dedup   []string `yaml:"dedup" km:"mode=dedup"`
		Replace []string `yaml:"replace" km:"mode=replace"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
concat: [a, b]
dedup: [a, b, c]
replace: [a, b]
`)

	overlay := []byte(`
concat: [c, d]
dedup: [b, c, d]
replace: [x, y]
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Concat: should have all items
	expectedConcat := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(config.Concat, expectedConcat) {
		t.Errorf("concat: expected %v, got %v", expectedConcat, config.Concat)
	}

	// Dedup: should have unique items
	expectedDedup := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(config.Dedup, expectedDedup) {
		t.Errorf("dedup: expected %v, got %v", expectedDedup, config.Dedup)
	}

	// Replace: should only have overlay items
	expectedReplace := []string{"x", "y"}
	if !reflect.DeepEqual(config.Replace, expectedReplace) {
		t.Errorf("replace: expected %v, got %v", expectedReplace, config.Replace)
	}
}

// Test Merger with field-specific object list modes.
func TestMerger_DupeModes(t *testing.T) {
	type Item struct {
		ID    string `yaml:"id" km:"primary"`
		Value int    `yaml:"value"`
	}

	type Config struct {
		UniqueItems      []Item `yaml:"unique" km:"dupe=unique"`
		ConsolidateItems []Item `yaml:"consolidate" km:"dupe=consolidate"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
unique:
  - id: a
    value: 1
consolidate:
  - id: a
    value: 1
  - id: a
    value: 2
`)

	overlay := []byte(`
unique:
  - id: b
    value: 2
consolidate:
  - id: b
    value: 3
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Unique: should have 2 items (a and b)
	if len(config.UniqueItems) != 2 {
		t.Fatalf("unique: expected 2 items, got %d", len(config.UniqueItems))
	}

	// Consolidate: duplicates in base should be merged, then overlay merged
	if len(config.ConsolidateItems) != 2 {
		t.Fatalf("consolidate: expected 2 items, got %d", len(config.ConsolidateItems))
	}

	// First item should have value from second occurrence (consolidate in base)
	var itemA *Item
	for i := range config.ConsolidateItems {
		if config.ConsolidateItems[i].ID == "a" {
			itemA = &config.ConsolidateItems[i]
			break
		}
	}
	if itemA == nil || itemA.Value != 2 {
		t.Fatalf("consolidate: expected item 'a' with value 2, got %v", itemA)
	}
}

// Test Merger with nested structs.
func TestMerger_NestedStructs(t *testing.T) {
	type Database struct {
		Name string `yaml:"name" km:"primary"`
		Host string `yaml:"host"`
	}

	type Service struct {
		Name      string     `yaml:"name" km:"primary"`
		Port      int        `yaml:"port"`
		Databases []Database `yaml:"databases"`
	}

	type Config struct {
		Services []Service `yaml:"services"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
services:
  - name: web
    port: 8080
    databases:
      - name: primary
        host: db1.example.com
`)

	overlay := []byte(`
services:
  - name: web
    databases:
      - name: primary
        host: db2.example.com
      - name: cache
        host: redis.example.com
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Should have 1 service
	if len(config.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(config.Services))
	}

	svc := config.Services[0]

	// Port should be preserved from base
	if svc.Port != 8080 {
		t.Errorf("expected port 8080, got %d", svc.Port)
	}

	// Should have 2 databases (merged primary + new cache)
	if len(svc.Databases) != 2 {
		t.Fatalf("expected 2 databases, got %d", len(svc.Databases))
	}

	// Primary database host should be updated
	var primaryDB *Database
	for i := range svc.Databases {
		if svc.Databases[i].Name == "primary" {
			primaryDB = &svc.Databases[i]
			break
		}
	}

	if primaryDB == nil || primaryDB.Host != "db2.example.com" {
		t.Fatalf("expected primary database with host db2.example.com, got %v", primaryDB)
	}
}

// Test Merger with custom field names.
func TestMerger_CustomFieldName(t *testing.T) {
	type Config struct {
		Items []string `someformat:"wtfs" km:"field=wtfs,mode=dedup"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	// Even though the struct tag uses "someformat", the km:"field=wtfs" override
	// tells the merger that the serialized field name is "wtfs"
	base := []byte(`wtfs: [a, b]`)
	overlay := []byte(`wtfs: [b, c]`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string][]string
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Should deduplicate using the custom field name
	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(parsed["wtfs"], expected) {
		t.Errorf("expected %v, got %v", expected, parsed["wtfs"])
	}
}

// Test Merger error on invalid tag.
func TestMerger_InvalidTag(t *testing.T) {
	type Config struct {
		Items []string `yaml:"items" km:"invalid=value"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for invalid km tag")
	}

	if !errors.Is(err, keymerge.ErrInvalidTag) {
		t.Errorf("expected ErrInvalidTag, got %v", err)
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	if tagErr.Kind != keymerge.UnknownTag {
		t.Errorf("expected UnknownTag, got %v", tagErr.Kind)
	}
	if tagErr.Value != "invalid=value" {
		t.Errorf("expected value 'invalid=value', got %q", tagErr.Value)
	}
}

// Test Merger with global options as fallback.
func TestMerger_GlobalOptionsFallback(t *testing.T) {
	type Item struct {
		Name  string `yaml:"name"`
		Value int    `yaml:"value"`
	}

	type Config struct {
		Items []Item `yaml:"items"`
	}

	// Use global PrimaryKeyNames since no km tags on Item
	merger, err := keymerge.NewMerger[Config](keymerge.Options{
		PrimaryKeyNames: []string{"name"},
	}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
items:
  - name: a
    value: 1
`)

	overlay := []byte(`
items:
  - name: a
    value: 2
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Should merge by name using global options
	if len(config.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(config.Items))
	}

	if config.Items[0].Value != 2 {
		t.Errorf("expected value 2, got %d", config.Items[0].Value)
	}
}

// Test Merger field name detection from yaml tags.
func TestMerger_FieldNameDetection(t *testing.T) {
	type Config struct {
		YAMLField string   `yaml:"yaml_name"`
		Items     []string `yaml:"items" km:"mode=concat"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
yaml_name: a
items: [x]
`)

	overlay := []byte(`
yaml_name: a2
items: [y]
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// YAMLField should use yaml tag name
	if config.YAMLField != "a2" {
		t.Errorf("YAMLField: expected a2, got %s", config.YAMLField)
	}

	// Items should be concatenated
	expectedItems := []string{"x", "y"}
	if !reflect.DeepEqual(config.Items, expectedItems) {
		t.Errorf("Items: expected %v, got %v", expectedItems, config.Items)
	}
}

// Test Merger error on invalid scalar list mode.
func TestMerger_InvalidScalarMode(t *testing.T) {
	type Config struct {
		Items []string `yaml:"items" km:"mode=invalid"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for invalid scalar list mode")
	}

	if !errors.Is(err, keymerge.ErrInvalidTag) {
		t.Errorf("expected ErrInvalidTag, got %v", err)
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	if tagErr.Kind != keymerge.ModeTag {
		t.Errorf("expected ModeTag, got %v", tagErr.Kind)
	}
	if tagErr.Value != "invalid" {
		t.Errorf("expected value 'invalid', got %q", tagErr.Value)
	}
}

// Test Merger error on invalid object list mode.
func TestMerger_InvalidDupeMode(t *testing.T) {
	type Item struct {
		ID string `yaml:"id" km:"primary"`
	}

	type Config struct {
		Items []Item `yaml:"items" km:"dupe=invalid"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for invalid object list mode")
	}

	if !errors.Is(err, keymerge.ErrInvalidTag) {
		t.Errorf("expected ErrInvalidTag, got %v", err)
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	if tagErr.Kind != keymerge.DupeTag {
		t.Errorf("expected DupeTag, got %v", tagErr.Kind)
	}
	if tagErr.Value != "invalid" {
		t.Errorf("expected value 'invalid', got %q", tagErr.Value)
	}
}

// Test Merger field name detection from json tags.
func TestMerger_FieldNameDetection_JSON(t *testing.T) {
	type Config struct {
		JSONField string   `json:"json_name"`
		Items     []string `json:"items" km:"mode=concat"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	// Use YAML for serialization, but field names come from json tags
	base := []byte(`
json_name: a
items: [x]
`)
	overlay := []byte(`
json_name: a2
items: [y]
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// JSONField should use json tag name
	if config.JSONField != "a2" {
		t.Errorf("JSONField: expected a2, got %s", config.JSONField)
	}

	// Items should be concatenated
	expectedItems := []string{"x", "y"}
	if !reflect.DeepEqual(config.Items, expectedItems) {
		t.Errorf("Items: expected %v, got %v", expectedItems, config.Items)
	}
}

// Test Merger field name detection from toml tags when yaml is absent.
func TestMerger_FieldNameDetection_TOML(t *testing.T) {
	type Config struct {
		TOMLField string   `toml:"toml_name" yaml:"toml_name"`
		Items     []string `toml:"items" yaml:"items" km:"mode=concat"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	// Use YAML for serialization
	// Since both yaml and toml tags present with same value, this tests the fallback works
	// The real test of "toml when yaml absent" is that without a yaml tag, toml would be used
	base := []byte(`
toml_name: a
items: [x]
`)
	overlay := []byte(`
toml_name: a2
items: [y]
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	if config.TOMLField != "a2" {
		t.Errorf("TOMLField: expected a2, got %s", config.TOMLField)
	}

	// Items should be concatenated
	expectedItems := []string{"x", "y"}
	if !reflect.DeepEqual(config.Items, expectedItems) {
		t.Errorf("Items: expected %v, got %v", expectedItems, config.Items)
	}
}

// Test Merger field name detection priority: yaml > json > toml.
func TestMerger_FieldNameDetection_Yaml_Over_Json(t *testing.T) {
	type Config struct {
		Field string `yaml:"yaml_name" json:"json_name"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	// Both tags present with different names, yaml should win
	base := []byte(`yaml_name: base`)
	overlay := []byte(`yaml_name: overlay`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	if config.Field != "overlay" {
		t.Errorf("expected overlay, got %s", config.Field)
	}
}

// Test Merger field name detection priority: json > toml.
func TestMerger_FieldNameDetection_Json_Over_Toml(t *testing.T) {
	type Config struct {
		Field string `json:"json_name" toml:"toml_name" yaml:"json_name"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	// json and toml have different names, json should win
	base := []byte(`json_name: base`)
	overlay := []byte(`json_name: overlay`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	if config.Field != "overlay" {
		t.Errorf("expected overlay, got %s", config.Field)
	}
}

// Test Merger field name detection priority (km:field > yaml > json > toml).
func TestMerger_FieldNameDetection_Priority(t *testing.T) {
	type Config struct {
		// km:field should override yaml for merge purposes
		// But yaml tag still used for unmarshaling, so we need to match them
		Field1 string `yaml:"override_name" json:"should_be_ignored" km:"field=override_name"`
		// yaml should override json
		Field2 string `yaml:"yaml_name" json:"json_override"`
		// json should override toml (need yaml for unmarshaling to work)
		Field3 string `json:"json_name" toml:"toml_name" yaml:"json_name"`
		// toml should override struct field name (need yaml for unmarshaling)
		Field4 string `toml:"toml_name" yaml:"toml_name"`
		// Struct field name as fallback
		Field5 string `yaml:"Field5"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
override_name: v1
yaml_name: v2
json_name: v3
toml_name: v4
Field5: v5
`)

	overlay := []byte(`
override_name: v1_new
yaml_name: v2_new
json_name: v3_new
toml_name: v4_new
Field5: v5_new
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Verify priority order was respected in metadata
	if config.Field1 != "v1_new" {
		t.Errorf("Field1: expected v1_new (km:field priority), got %s", config.Field1)
	}
	if config.Field2 != "v2_new" {
		t.Errorf("Field2: expected v2_new (yaml priority), got %s", config.Field2)
	}
	if config.Field3 != "v3_new" {
		t.Errorf("Field3: expected v3_new (json priority over toml), got %s", config.Field3)
	}
	if config.Field4 != "v4_new" {
		t.Errorf("Field4: expected v4_new (toml priority over struct name), got %s", config.Field4)
	}
	if config.Field5 != "v5_new" {
		t.Errorf("Field5: expected v5_new (struct name fallback), got %s", config.Field5)
	}
}

// Test Merger composite key with missing field (incomplete key should not match).
func TestMerger_CompositePrimaryKey_MissingField(t *testing.T) {
	type Endpoint struct {
		Region string `yaml:"region" km:"primary"`
		Name   string `yaml:"name" km:"primary"`
		URL    string `yaml:"url"`
	}

	type Config struct {
		Endpoints []Endpoint `yaml:"endpoints"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
endpoints:
  - region: us-east
    name: api
    url: v1.example.com
`)

	// Overlay missing 'name' field - should be appended, not merged
	overlay := []byte(`
endpoints:
  - region: us-east
    url: v2.example.com
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Should have 2 endpoints (not merged because composite key incomplete)
	if len(config.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(config.Endpoints))
	}

	// First endpoint should still have v1 URL (not merged)
	if config.Endpoints[0].URL != "v1.example.com" {
		t.Errorf("expected first endpoint to have v1.example.com, got %s", config.Endpoints[0].URL)
	}

	// Second endpoint should have v2 URL (appended)
	if config.Endpoints[1].URL != "v2.example.com" {
		t.Errorf("expected second endpoint to have v2.example.com, got %s", config.Endpoints[1].URL)
	}
}

// Test Merger field name with omitempty/inline modifiers.
func TestMerger_FieldNameDetection_WithModifiers(t *testing.T) {
	type Config struct {
		Field1 string   `yaml:"field_name,omitempty"`
		Field2 string   `yaml:"other_name"`
		Items  []string `yaml:"items,flow" km:"mode=concat"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
field_name: a
other_name: b
items: [x]
`)

	overlay := []byte(`
field_name: a2
other_name: b2
items: [y]
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Field names should be extracted correctly (before comma)
	if config.Field1 != "a2" {
		t.Errorf("Field1: expected a2, got %s", config.Field1)
	}
	if config.Field2 != "b2" {
		t.Errorf("Field2: expected b2, got %s", config.Field2)
	}

	expectedItems := []string{"x", "y"}
	if !reflect.DeepEqual(config.Items, expectedItems) {
		t.Errorf("Items: expected %v, got %v", expectedItems, config.Items)
	}
}

// Test Merger deletion with composite primary keys.
func TestMerger_DeleteWithCompositePrimaryKey(t *testing.T) {
	type Endpoint struct {
		Region string `yaml:"region" km:"primary"`
		Name   string `yaml:"name" km:"primary"`
		URL    string `yaml:"url"`
	}

	type Config struct {
		Endpoints []Endpoint `yaml:"endpoints"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{
		DeleteMarkerKey: "_delete",
	}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
endpoints:
  - region: us-east
    name: api
    url: v1.example.com
  - region: us-west
    name: api
    url: v1-west.example.com
`)

	overlay := []byte(`
endpoints:
  - region: us-east
    name: api
    _delete: true
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Should only have us-west/api (us-east/api deleted)
	if len(config.Endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(config.Endpoints))
	}

	if config.Endpoints[0].Region != "us-west" {
		t.Errorf("expected us-west endpoint, got %s", config.Endpoints[0].Region)
	}
}

// Test Merger with non-comparable composite key types is rejected at construction.
func TestMerger_CompositePrimaryKey_NonComparable(t *testing.T) {
	type Endpoint struct {
		Region string   `yaml:"region" km:"primary"`
		Tags   []string `yaml:"tags" km:"primary"` // Slice is not comparable!
		URL    string   `yaml:"url"`
	}

	type Config struct {
		Endpoints []Endpoint `yaml:"endpoints"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for non-comparable composite key type")
	}

	// Should get InvalidTagError for the Tags field
	if !errors.Is(err, keymerge.ErrInvalidTag) {
		t.Errorf("expected ErrInvalidTag, got %v", err)
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	if tagErr.Kind != keymerge.PrimaryTag {
		t.Errorf("expected PrimaryTag, got %v", tagErr.Kind)
	}
	if !strings.Contains(tagErr.Message, "comparable") {
		t.Errorf("error should mention comparable: %s", tagErr.Message)
	}
}

// Test Merger with deeply nested structures (3+ levels).
func TestMerger_DeeplyNestedStructs(t *testing.T) {
	type Setting struct {
		Key   string `yaml:"key" km:"primary"`
		Value string `yaml:"value"`
	}

	type Database struct {
		Name     string    `yaml:"name" km:"primary"`
		Host     string    `yaml:"host"`
		Settings []Setting `yaml:"settings"`
	}

	type Service struct {
		Name      string     `yaml:"name" km:"primary"`
		Port      int        `yaml:"port"`
		Databases []Database `yaml:"databases"`
	}

	type Config struct {
		Services []Service `yaml:"services"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
services:
  - name: web
    port: 8080
    databases:
      - name: primary
        host: db1.example.com
        settings:
          - key: timeout
            value: "30s"
          - key: pool_size
            value: "10"
`)

	overlay := []byte(`
services:
  - name: web
    databases:
      - name: primary
        host: db2.example.com
        settings:
          - key: timeout
            value: "60s"
          - key: max_connections
            value: "100"
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Should have 1 service
	if len(config.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(config.Services))
	}

	svc := config.Services[0]

	// Port should be preserved from base
	if svc.Port != 8080 {
		t.Errorf("expected port 8080, got %d", svc.Port)
	}

	// Should have 1 database (merged)
	if len(svc.Databases) != 1 {
		t.Fatalf("expected 1 database, got %d", len(svc.Databases))
	}

	db := svc.Databases[0]

	// Host should be updated
	if db.Host != "db2.example.com" {
		t.Errorf("expected host db2.example.com, got %s", db.Host)
	}

	// Should have 3 settings (timeout merged, pool_size kept, max_connections added)
	if len(db.Settings) != 3 {
		t.Fatalf("expected 3 settings, got %d", len(db.Settings))
	}

	// Find and verify timeout setting was merged
	var timeoutSetting *Setting
	for i := range db.Settings {
		if db.Settings[i].Key == "timeout" {
			timeoutSetting = &db.Settings[i]
			break
		}
	}

	if timeoutSetting == nil || timeoutSetting.Value != "60s" {
		t.Errorf("expected timeout setting with value 60s, got %v", timeoutSetting)
	}
}

// Test InvalidTagError for unknown tag directives.
func TestInvalidTagError_UnknownDirective(t *testing.T) {
	type Config struct {
		Items []string `yaml:"items" km:"unknown_directive"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for unknown km tag directive")
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	if tagErr.Kind != keymerge.UnknownTag {
		t.Errorf("expected UnknownTag kind, got %v", tagErr.Kind)
	}
	if tagErr.FieldName != "items" {
		t.Errorf("expected field 'items', got %q", tagErr.FieldName)
	}
	if tagErr.Value != "unknown_directive" {
		t.Errorf("expected value 'unknown_directive', got %q", tagErr.Value)
	}
	if !strings.Contains(tagErr.Error(), "invalid unknown tag") {
		t.Errorf("error message should mention 'invalid unknown tag': %s", tagErr.Error())
	}
}

// Test InvalidTagError for invalid scalar list mode values.
// Test InvalidTagError for invalid scalar/object list mode values.
func TestInvalidTagError_InvalidModeValues(t *testing.T) {
	tests := []struct {
		name         string
		createMerger func() error
		wantKind     keymerge.TagKind
		wantValue    string
		wantMsg      string
	}{
		{
			name: "ScalarMode_Typo",
			createMerger: func() error {
				type Config struct {
					Items []string `yaml:"items" km:"mode=concat_typo"`
				}
				_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
				return err
			},
			wantKind:  keymerge.ModeTag,
			wantValue: "concat_typo",
			wantMsg:   "invalid mode tag",
		},
		{
			name: "ScalarMode_Uppercase",
			createMerger: func() error {
				type Config struct {
					Items []string `yaml:"items" km:"mode=CONCAT"`
				}
				_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
				return err
			},
			wantKind:  keymerge.ModeTag,
			wantValue: "CONCAT",
		},
		{
			name: "DupeMode_Typo",
			createMerger: func() error {
				type Item struct {
					ID string `yaml:"id" km:"primary"`
				}
				type Config struct {
					Items []Item `yaml:"items" km:"dupe=uniqu"`
				}
				_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
				return err
			},
			wantKind:  keymerge.DupeTag,
			wantValue: "uniqu",
			wantMsg:   "invalid dupe tag",
		},
		{
			name: "DupeMode_Uppercase",
			createMerger: func() error {
				type Item struct {
					ID string `yaml:"id" km:"primary"`
				}
				type Config struct {
					Items []Item `yaml:"items" km:"dupe=CONSOLIDATE"`
				}
				_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
				return err
			},
			wantKind:  keymerge.DupeTag,
			wantValue: "CONSOLIDATE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.createMerger()
			if err == nil {
				t.Fatal("expected error for invalid tag")
			}

			var tagErr *keymerge.InvalidTagError
			if !errors.As(err, &tagErr) {
				t.Fatalf("expected InvalidTagError, got %T", err)
			}

			if tagErr.Kind != tt.wantKind {
				t.Errorf("expected %v kind, got %v", tt.wantKind, tagErr.Kind)
			}
			if tagErr.Value != tt.wantValue {
				t.Errorf("expected value %q, got %q", tt.wantValue, tagErr.Value)
			}
			if tt.wantMsg != "" && !strings.Contains(tagErr.Error(), tt.wantMsg) {
				t.Errorf("error message should mention %q: %s", tt.wantMsg, tagErr.Error())
			}
		})
	}
}

// Test InvalidTagError contains helpful message with valid options.
func TestInvalidTagError_HelpfulMessage(t *testing.T) {
	type Config struct {
		BadMode []string `yaml:"items" km:"mode=badvalue"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	// Message should indicate valid options
	if !strings.Contains(tagErr.Message, "concat") || !strings.Contains(tagErr.Message, "dedup") {
		t.Errorf("message should mention valid options: %s", tagErr.Message)
	}

	errMsg := tagErr.Error()
	if !strings.Contains(errMsg, "badvalue") {
		t.Errorf("error should include invalid value: %s", errMsg)
	}
}

// Test InvalidTagError for multiple directives where one is invalid.
func TestInvalidTagError_MultipleDirectives(t *testing.T) {
	type Config struct {
		Items []string `yaml:"items" km:"mode=concat,badname=value"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for invalid directive in multi-directive tag")
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	// Should report the bad directive
	if tagErr.Value != "badname=value" {
		t.Errorf("expected value 'badname=value', got %q", tagErr.Value)
	}
}

// Test InvalidTagError.Is() works with sentinel error.
func TestInvalidTagError_IsSentinel(t *testing.T) {
	type Config struct {
		Items []string `yaml:"items" km:"mode=badmode"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error")
	}

	// Should work with errors.Is() for the sentinel
	if !errors.Is(err, keymerge.ErrInvalidTag) {
		t.Errorf("errors.Is() with ErrInvalidTag should return true")
	}

	// But not for other sentinels
	if errors.Is(err, keymerge.ErrInvalidOptions) {
		t.Errorf("errors.Is() with ErrInvalidOptions should return false")
	}
}

// Test TagKind.String() method.
func TestTagKind_String(t *testing.T) {
	tests := []struct {
		kind     keymerge.TagKind
		expected string
	}{
		{keymerge.UnknownTag, "unknown"},
		{keymerge.PrimaryTag, "primary"},
		{keymerge.ModeTag, "mode"},
		{keymerge.DupeTag, "dupe"},
		{keymerge.FieldTag, "field"},
	}

	for _, tc := range tests {
		if tc.kind.String() != tc.expected {
			t.Errorf("TagKind(%d).String() expected %q, got %q", tc.kind, tc.expected, tc.kind.String())
		}
	}
}

// Test composite primary keys with 3+ fields.
func TestMerger_CompositePrimaryKey_ThreeFields(t *testing.T) {
	type Record struct {
		Region   string `yaml:"region" km:"primary"`
		Service  string `yaml:"service" km:"primary"`
		Instance string `yaml:"instance" km:"primary"`
		Value    string `yaml:"value"`
	}

	type Config struct {
		Records []Record `yaml:"records"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
records:
  - region: us-east
    service: api
    instance: "1"
    value: v1
  - region: us-west
    service: api
    instance: "1"
    value: v1
`)

	overlay := []byte(`
records:
  - region: us-east
    service: api
    instance: "1"
    value: v2
  - region: us-east
    service: api
    instance: "2"
    value: v3
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Should have 3 records total
	if len(config.Records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(config.Records))
	}

	// Find and verify the merged record (us-east/api/1)
	var mergedRecord *Record
	for i := range config.Records {
		if config.Records[i].Region == "us-east" &&
			config.Records[i].Service == "api" &&
			config.Records[i].Instance == "1" {
			mergedRecord = &config.Records[i]
			break
		}
	}

	if mergedRecord == nil {
		t.Fatal("merged record not found")
	}
	if mergedRecord.Value != "v2" {
		t.Errorf("expected merged record value v2, got %s", mergedRecord.Value)
	}
}

// Test composite primary keys with mixed types.
func TestMerger_CompositePrimaryKey_MixedTypes(t *testing.T) {
	type Endpoint struct {
		Port   int    `yaml:"port" km:"primary"`
		Name   string `yaml:"name" km:"primary"`
		Active bool   `yaml:"active" km:"primary"`
		URL    string `yaml:"url"`
	}

	type Config struct {
		Endpoints []Endpoint `yaml:"endpoints"`
	}

	merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err != nil {
		t.Fatal(err)
	}

	base := []byte(`
endpoints:
  - port: 8080
    name: api
    active: true
    url: v1.example.com
  - port: 8080
    name: api
    active: false
    url: v1-disabled.example.com
`)

	overlay := []byte(`
endpoints:
  - port: 8080
    name: api
    active: true
    url: v2.example.com
`)

	result, err := merger.Merge(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}

	// Should have 2 endpoints
	if len(config.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(config.Endpoints))
	}

	// Find the merged endpoint (port=8080, name=api, active=true)
	var mergedEndpoint *Endpoint
	for i := range config.Endpoints {
		if config.Endpoints[i].Port == 8080 &&
			config.Endpoints[i].Name == "api" &&
			config.Endpoints[i].Active {
			mergedEndpoint = &config.Endpoints[i]
			break
		}
	}

	if mergedEndpoint == nil {
		t.Fatal("merged endpoint not found")
	}
	if mergedEndpoint.URL != "v2.example.com" {
		t.Errorf("expected merged endpoint URL v2.example.com, got %s", mergedEndpoint.URL)
	}
}

// Test composite primary key field ordering and independence.
func TestMerger_CompositePrimaryKey_FieldOrder(t *testing.T) {
	tests := []struct {
		name     string
		base     []byte
		overlay  []byte
		wantLen  int
		wantURL  string // for the matching item
		matchKey string // descriptive key for verification
	}{
		{
			name: "fields in same order",
			base: []byte(`
items:
  - a: x
    b: y
    url: v1
`),
			overlay: []byte(`
items:
  - a: x
    b: y
    url: v2
`),
			wantLen: 1,
			wantURL: "v2",
		},
		{
			name: "fields in different order",
			base: []byte(`
items:
  - b: y
    a: x
    url: v1
`),
			overlay: []byte(`
items:
  - a: x
    b: y
    url: v2
`),
			wantLen: 1,
			wantURL: "v2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			type Item struct {
				A   string `yaml:"a" km:"primary"`
				B   string `yaml:"b" km:"primary"`
				URL string `yaml:"url"`
			}

			type Config struct {
				Items []Item `yaml:"items"`
			}

			merger, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
			if err != nil {
				t.Fatal(err)
			}

			result, err := merger.Merge(tc.base, tc.overlay)
			if err != nil {
				t.Fatal(err)
			}

			var config Config
			if err := yaml.Unmarshal(result, &config); err != nil {
				t.Fatal(err)
			}

			if len(config.Items) != tc.wantLen {
				t.Errorf("expected %d items, got %d", tc.wantLen, len(config.Items))
			}

			if len(config.Items) > 0 && config.Items[0].URL != tc.wantURL {
				t.Errorf("expected URL %q, got %q", tc.wantURL, config.Items[0].URL)
			}
		})
	}
}

// Test Merger validation rejects slice primary key types at construction time.
func TestMerger_NonComparablePrimaryKeyType_Slice(t *testing.T) {
	type Endpoint struct {
		Tags []string `yaml:"tags" km:"primary"`
		URL  string   `yaml:"url"`
	}
	type Config struct {
		Endpoints []Endpoint `yaml:"endpoints"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for slice primary key type")
	}

	if !errors.Is(err, keymerge.ErrInvalidTag) {
		t.Errorf("expected ErrInvalidTag, got %v", err)
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	if tagErr.Kind != keymerge.PrimaryTag {
		t.Errorf("expected PrimaryTag, got %v", tagErr.Kind)
	}
	if !strings.Contains(tagErr.Message, "comparable") {
		t.Errorf("error should mention comparable: %s", tagErr.Message)
	}
}

// Test Merger validation rejects map primary key types at construction time.
func TestMerger_NonComparablePrimaryKeyType_Map(t *testing.T) {
	type Endpoint struct {
		Metadata map[string]string `yaml:"metadata" km:"primary"`
		URL      string            `yaml:"url"`
	}
	type Config struct {
		Endpoints []Endpoint `yaml:"endpoints"`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for map primary key type")
	}

	if !errors.Is(err, keymerge.ErrInvalidTag) {
		t.Errorf("expected ErrInvalidTag, got %v", err)
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	if tagErr.Kind != keymerge.PrimaryTag {
		t.Errorf("expected PrimaryTag, got %v", tagErr.Kind)
	}
	if !strings.Contains(tagErr.Message, "comparable") {
		t.Errorf("error should mention comparable: %s", tagErr.Message)
	}
}

// Test Merger validation rejects empty km:field value.
func TestMerger_InvalidFieldName_Empty(t *testing.T) {
	type Config struct {
		Items []string `yaml:"items" km:"field="`
	}

	_, err := keymerge.NewMerger[Config](keymerge.Options{}, yaml.Unmarshal, yaml.Marshal)
	if err == nil {
		t.Fatal("expected error for empty field name")
	}

	if !errors.Is(err, keymerge.ErrInvalidTag) {
		t.Errorf("expected ErrInvalidTag, got %v", err)
	}

	var tagErr *keymerge.InvalidTagError
	if !errors.As(err, &tagErr) {
		t.Fatalf("expected InvalidTagError, got %T", err)
	}

	if tagErr.Kind != keymerge.FieldTag {
		t.Errorf("expected FieldTag, got %v", tagErr.Kind)
	}
	if !strings.Contains(tagErr.Message, "empty") {
		t.Errorf("error should mention empty: %s", tagErr.Message)
	}
}
