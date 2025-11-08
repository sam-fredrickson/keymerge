// SPDX-License-Identifier: Apache-2.0

package keymerge_test

import (
	_ "embed"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/sam-fredrickson/keymerge"
)

// Test helpers for YAML-specific merging.
func mergeYAML(docs ...[]byte) ([]byte, error) {
	return keymerge.MergeMarshal(
		keymerge.Options{
			PrimaryKeyNames: []string{"name", "id"},
		},
		yaml.Unmarshal, yaml.Marshal, docs...)
}

func mergeYAMLWith(opts keymerge.Options, docs ...[]byte) ([]byte, error) {
	return keymerge.MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, docs...)
}

type testConfig struct {
	Foos []fooConfig `yaml:"foos"`
}

type fooConfig struct {
	Name  string `yaml:"name"`
	Type  string `yaml:"type"`
	Count int    `yaml:"count"`
}

//go:embed testfiles/foo-base.yaml
var fooBase []byte

//go:embed testfiles/foo-o1.yaml
var fooOverlay1 []byte

//go:embed testfiles/foo-o2.yaml
var fooOverlay2 []byte

//go:embed testfiles/foo-z.yaml
var fooFinal []byte

func TestSmoke(t *testing.T) {
	parse := func(raw []byte) testConfig {
		var cfg testConfig
		if err := yaml.Unmarshal(raw, &cfg); err != nil {
			t.Fatal(err)
		}
		return cfg
	}

	actualFooFinal, err := mergeYAML(fooBase, fooOverlay1, fooOverlay2)
	if err != nil {
		t.Fatal(err)
	}

	actual := parse(actualFooFinal)
	expected := parse(fooFinal)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("actual:\n%v\nexpected:\n%v", actual, expected)
	}
}

func TestEmptyDocs(t *testing.T) {
	result, err := mergeYAML()
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty result, got: %s", result)
	}
}

func TestSlicesWithoutPrimaryKeys(t *testing.T) {
	base := []byte(`values: [1, 2, 3]`)
	overlay := []byte(`values: [4, 5]`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string][]int
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Should append since no primary keys
	expected := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(parsed["values"], expected) {
		t.Fatalf("expected %v, got %v", expected, parsed["values"])
	}
}

func TestScalarOverride(t *testing.T) {
	base := []byte(`
name: foo
count: 10
enabled: true
`)
	overlay := []byte(`
count: 20
enabled: false
`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Name    string `yaml:"name"`
		Count   int    `yaml:"count"`
		Enabled bool   `yaml:"enabled"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	if parsed.Name != "foo" || parsed.Count != 20 || parsed.Enabled != false {
		t.Fatalf("unexpected values: %+v", parsed)
	}
}

func TestEmptyOverlaySlice(t *testing.T) {
	base := []byte(`foos:
  - name: foo1
    count: 1
`)
	overlay := []byte(`foos: []`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed testConfig
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Empty overlay should return base
	if len(parsed.Foos) != 1 {
		t.Fatalf("expected 1 foo, got %d", len(parsed.Foos))
	}
}

func TestItemWithoutPrimaryKey(t *testing.T) {
	base := []byte(`
items:
  - name: item1
    value: 1
`)
	overlay := []byte(`
items:
  - value: 2
`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string][]map[string]any
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Item without primary key should be appended
	if len(parsed["items"]) != 2 {
		t.Fatalf("expected 2 items, got %d", len(parsed["items"]))
	}
}

func TestNonMapItemsInSlice(t *testing.T) {
	base := []byte(`items: ["a", "b"]`)
	overlay := []byte(`items: ["c"]`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string][]string
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Non-map items should be appended
	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(parsed["items"], expected) {
		t.Fatalf("expected %v, got %v", expected, parsed["items"])
	}
}

func TestInvalidYAML(t *testing.T) {
	base := []byte(`invalid: yaml: [`)
	overlay := []byte(`foo: bar`)

	_, err := mergeYAML(base, overlay)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}

	if !errors.Is(err, keymerge.ErrMarshal) {
		t.Errorf("expected errors.Is(err, ErrMarshal) to be true")
	}
}

func TestAlternativePrimaryKey(t *testing.T) {
	base := []byte(`
users:
  - id: 1
    name: alice
  - id: 2
    name: bob
`)
	overlay := []byte(`
users:
  - id: 1
    name: alice_updated
  - id: 3
    name: charlie
`)

	result, err := mergeYAMLWith(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Users []struct {
			ID   int    `yaml:"id"`
			Name string `yaml:"name"`
		} `yaml:"users"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	if len(parsed.Users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(parsed.Users))
	}

	// Check that id:1 was updated
	if parsed.Users[0].Name != "alice_updated" {
		t.Fatalf("expected alice_updated, got %s", parsed.Users[0].Name)
	}
}

func TestNilOverlay(t *testing.T) {
	base := []byte(`foo: bar`)
	overlay := []byte(`foo: null`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Nil overlay should keep base
	if parsed["foo"] != "bar" {
		t.Fatalf("expected 'bar', got %v", parsed["foo"])
	}
}

func TestNilBase(t *testing.T) {
	base := []byte(`foo: null`)
	overlay := []byte(`foo: bar`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Overlay should replace nil base
	if parsed["foo"] != "bar" {
		t.Fatalf("expected 'bar', got %v", parsed["foo"])
	}
}

func TestMapWithNewKeys(t *testing.T) {
	base := []byte(`
a: 1
b: 2
`)
	overlay := []byte(`
c: 3
d: 4
`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]int
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	expected := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4}
	if !reflect.DeepEqual(parsed, expected) {
		t.Fatalf("expected %v, got %v", expected, parsed)
	}
}

func TestMixedMapAndNonMapItemsInSlice(t *testing.T) {
	base := []byte(`
items:
  - name: item1
    value: 1
`)
	overlay := []byte(`
items:
  - name: item2
    value: 2
  - "string_item"
`)

	result, err := mergeYAML(base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string][]any
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Should have 3 items: item1 (base), item2 (overlay), and string_item (overlay, appended)
	if len(parsed["items"]) != 3 {
		t.Fatalf("expected 3 items, got %d", len(parsed["items"]))
	}
}

func TestDeleteMapKey(t *testing.T) {
	base := []byte(`
settings:
  debug: true
  timeout: 30
  retries: 3
`)
	overlay := []byte(`
settings:
  timeout:
    _delete: true
  retries: 5
`)

	result, err := mergeYAMLWith(keymerge.Options{
		DeleteMarkerKey: "_delete",
		PrimaryKeyNames: []string{"name", "id"},
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Settings map[string]any `yaml:"settings"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// timeout should be deleted
	if _, exists := parsed.Settings["timeout"]; exists {
		t.Fatal("expected timeout to be deleted")
	}

	// debug should still exist
	if parsed.Settings["debug"] != true {
		t.Fatalf("expected debug=true, got %v", parsed.Settings["debug"])
	}

	// retries should be updated
	if retriesVal, ok := parsed.Settings["retries"].(uint64); !ok || retriesVal != 5 {
		t.Fatalf("expected retries=5, got %v", parsed.Settings["retries"])
	}
}

func TestDeleteListItem(t *testing.T) {
	base := []byte(`
users:
  - name: alice
    role: admin
  - name: bob
    role: user
  - name: charlie
    role: user
`)
	overlay := []byte(`
users:
  - name: bob
    _delete: true
  - name: charlie
    role: admin
`)

	result, err := mergeYAMLWith(keymerge.Options{
		DeleteMarkerKey: "_delete",
		PrimaryKeyNames: []string{"name"},
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Users []struct {
			Name string `yaml:"name"`
			Role string `yaml:"role"`
		} `yaml:"users"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Should have 2 users (bob deleted)
	if len(parsed.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(parsed.Users))
	}

	// Check alice is still there
	if parsed.Users[0].Name != "alice" || parsed.Users[0].Role != "admin" {
		t.Fatalf("expected alice with admin role, got %v", parsed.Users[0])
	}

	// Check charlie is still there and updated
	if parsed.Users[1].Name != "charlie" || parsed.Users[1].Role != "admin" {
		t.Fatalf("expected charlie with admin role, got %v", parsed.Users[1])
	}
}

func TestDeleteNonExistentItem(t *testing.T) {
	base := []byte(`
users:
  - name: alice
    role: admin
`)
	overlay := []byte(`
users:
  - name: bob
    _delete: true
`)

	result, err := mergeYAMLWith(keymerge.Options{
		DeleteMarkerKey: "_delete",
		PrimaryKeyNames: []string{"name"},
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Users []struct {
			Name string `yaml:"name"`
			Role string `yaml:"role"`
		} `yaml:"users"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Should still have alice (bob didn't exist)
	if len(parsed.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(parsed.Users))
	}

	if parsed.Users[0].Name != "alice" {
		t.Fatalf("expected alice, got %s", parsed.Users[0].Name)
	}
}

func TestDeleteMarkerFalse(t *testing.T) {
	base := []byte(`
users:
  - name: alice
    role: admin
`)
	overlay := []byte(`
users:
  - name: alice
    _delete: false
    role: user
`)

	result, err := mergeYAMLWith(keymerge.Options{
		DeleteMarkerKey: "_delete",
		PrimaryKeyNames: []string{"name"},
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Users []struct {
			Name string `yaml:"name"`
			Role string `yaml:"role"`
		} `yaml:"users"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Alice should be updated, not deleted
	if len(parsed.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(parsed.Users))
	}

	if parsed.Users[0].Role != "user" {
		t.Fatalf("expected role=user, got %s", parsed.Users[0].Role)
	}
}

func verifyStringTags(t *testing.T, result []byte, expected []string) {
	t.Helper()
	var parsed struct {
		Tags []string `yaml:"tags"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.Tags, expected) {
		t.Fatalf("expected %v, got %v", expected, parsed.Tags)
	}
}

func verifyIntPorts(t *testing.T, result []byte, expected []int) {
	t.Helper()
	var parsed struct {
		Ports []int `yaml:"ports"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed.Ports, expected) {
		t.Fatalf("expected %v, got %v", expected, parsed.Ports)
	}
}

func TestScalarListModes(t *testing.T) {
	tests := []struct {
		name         string
		mode         keymerge.ScalarListMode
		base         string
		overlay      string
		expectedTags []string
		expectedInts []int
	}{
		{
			name:         "Concat",
			mode:         keymerge.ScalarListConcat,
			base:         `tags: [foo, bar]`,
			overlay:      `tags: [baz, qux]`,
			expectedTags: []string{"foo", "bar", "baz", "qux"},
		},
		{
			name:         "Dedup",
			mode:         keymerge.ScalarListDedup,
			base:         `tags: [foo, bar, baz]`,
			overlay:      `tags: [bar, qux, foo]`,
			expectedTags: []string{"foo", "bar", "baz", "qux"},
		},
		{
			name:         "Replace",
			mode:         keymerge.ScalarListReplace,
			base:         `tags: [foo, bar, baz]`,
			overlay:      `tags: [qux, quux]`,
			expectedTags: []string{"qux", "quux"},
		},
		{
			name:         "DedupNumbers",
			mode:         keymerge.ScalarListDedup,
			base:         `ports: [8080, 8081, 8082]`,
			overlay:      `ports: [8081, 8083, 8080]`,
			expectedInts: []int{8080, 8081, 8082, 8083},
		},
		{
			name:         "DefaultIsConcat",
			mode:         keymerge.ScalarListConcat, // Explicitly set to show it's the default
			base:         `tags: [a, b]`,
			overlay:      `tags: [c]`,
			expectedTags: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := keymerge.Options{
				ScalarListMode: tt.mode,
			}
			// Add PrimaryKeyNames for non-number tests to match original behavior
			if tt.expectedTags != nil {
				opts.PrimaryKeyNames = []string{"name"}
			}

			result, err := mergeYAMLWith(opts, []byte(tt.base), []byte(tt.overlay))
			if err != nil {
				t.Fatal(err)
			}

			// Parse and verify based on expected type
			if tt.expectedTags != nil {
				verifyStringTags(t, result, tt.expectedTags)
				return
			}
			if tt.expectedInts != nil {
				verifyIntPorts(t, result, tt.expectedInts)
			}
		})
	}
}

func TestScalarListMode_DefaultIsConcat(t *testing.T) {
	base := []byte(`tags: [a, b]`)
	overlay := []byte(`tags: [c]`)

	// Don't specify ScalarListMode at all, should default to concat
	result, err := mergeYAMLWith(keymerge.Options{}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Tags []string `yaml:"tags"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	expected := []string{"a", "b", "c"}
	if !reflect.DeepEqual(parsed.Tags, expected) {
		t.Fatalf("expected %v, got %v", expected, parsed.Tags)
	}
}

func TestDeleteMarkerNonBoolValue(t *testing.T) {
	base := []byte(`
users:
  - name: alice
    role: admin
`)
	overlay := []byte(`
users:
  - name: alice
    _delete: "not a bool"
    role: user
`)

	result, err := mergeYAMLWith(keymerge.Options{
		DeleteMarkerKey: "_delete",
		PrimaryKeyNames: []string{"name"},
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Users []struct {
			Name string `yaml:"name"`
			Role string `yaml:"role"`
		} `yaml:"users"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Alice should be updated, not deleted (marker is not bool true)
	if len(parsed.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(parsed.Users))
	}

	if parsed.Users[0].Role != "user" {
		t.Fatalf("expected role=user, got %s", parsed.Users[0].Role)
	}
}

func TestScalarListMode_DedupComplexTypes(t *testing.T) {
	// Test dedup with maps and slices (should not deduplicate, always add)
	base := map[string]any{
		"items": []any{
			map[string]any{"x": 1},
			map[string]any{"x": 1}, // Same content but different instance
		},
	}
	overlay := map[string]any{
		"items": []any{
			map[string]any{"x": 1}, // Another instance
		},
	}

	result, err := keymerge.Merge(keymerge.Options{
		ScalarListMode: keymerge.ScalarListDedup,
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	resultMap := result.(map[string]any)
	items := resultMap["items"].([]any)

	// Maps aren't comparable, so all 3 should be present (no deduplication)
	if len(items) != 3 {
		t.Fatalf("expected 3 items (maps not deduplicated), got %d", len(items))
	}
}

func TestDeleteMarkersAreStripped(t *testing.T) {
	base := []byte(`
users:
  - name: alice
    role: admin
  - name: bob
    role: user
`)
	overlay := []byte(`
users:
  - name: alice
    _delete: false
    role: superadmin
  - name: charlie
    _delete: false
    role: guest
`)

	result, err := mergeYAMLWith(keymerge.Options{
		DeleteMarkerKey: "_delete",
		PrimaryKeyNames: []string{"name"},
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Users []map[string]any `yaml:"users"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Verify delete markers are not present in any user
	for i, user := range parsed.Users {
		if _, hasDeleteMarker := user["_delete"]; hasDeleteMarker {
			t.Fatalf("user %d still has _delete marker: %v", i, user)
		}
	}

	// Verify the data is correct
	if len(parsed.Users) != 3 {
		t.Fatalf("expected 3 users, got %d", len(parsed.Users))
	}
}

func TestObjectListMode_UniqueErrorsOnDuplicateInBase(t *testing.T) {
	base := []byte(`
users:
  - id: alice
    role: user
  - id: bob
    role: admin
  - id: alice
    role: manager
`)
	overlay := []byte(`
users:
  - id: charlie
    role: user
`)

	_, err := mergeYAMLWith(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
		ObjectListMode:  keymerge.ObjectListUnique,
	}, base, overlay)

	if err == nil {
		t.Fatal("expected error for duplicate keys in base, got nil")
	}

	if !errors.Is(err, keymerge.ErrDuplicatePrimaryKey) {
		t.Errorf("expected errors.Is(err, ErrDuplicatePrimaryKey) to be true")
	}

	var dupErr *keymerge.DuplicatePrimaryKeyError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicatePrimaryKeyError, got %T: %v", err, err)
	}

	if dupErr.Key != "alice" {
		t.Fatalf("expected duplicate key 'alice', got %v", dupErr.Key)
	}

	if len(dupErr.Positions) != 2 || dupErr.Positions[0] != 0 || dupErr.Positions[1] != 2 {
		t.Fatalf("expected positions [0, 2], got %v", dupErr.Positions)
	}

	// Path should be either users.0 or users.2 (the duplicate positions)
	if !slices.Equal(dupErr.Path, []string{"users", "0"}) && !slices.Equal(dupErr.Path, []string{"users", "2"}) {
		t.Fatalf("expected duplicate path 'users.0' or 'users.2', got %v", dupErr.Path)
	}
}

func TestObjectListMode_UniqueErrorsOnDuplicateInOverlay(t *testing.T) {
	base := []byte(`
users:
  - id: alice
    role: user
`)
	overlay := []byte(`
users:
  - id: bob
    role: admin
  - id: charlie
    role: user
  - id: bob
    role: manager
`)

	_, err := mergeYAMLWith(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
		ObjectListMode:  keymerge.ObjectListUnique,
	}, base, overlay)

	if err == nil {
		t.Fatal("expected error for duplicate keys in overlay, got nil")
	}

	if !errors.Is(err, keymerge.ErrDuplicatePrimaryKey) {
		t.Errorf("expected errors.Is(err, ErrDuplicatePrimaryKey) to be true")
	}

	var dupErr *keymerge.DuplicatePrimaryKeyError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicatePrimaryKeyError, got %T: %v", err, err)
	}

	if dupErr.Key != "bob" {
		t.Fatalf("expected duplicate key 'bob', got %v", dupErr.Key)
	}

	// Path should be either users.0 or users.2 (the duplicate positions in overlay)
	if !slices.Equal(dupErr.Path, []string{"users", "0"}) && !slices.Equal(dupErr.Path, []string{"users", "2"}) {
		t.Fatalf("expected duplicate path 'users.0' or 'users.2', got %v", dupErr.Path)
	}
}

func TestObjectListMode_ConsolidateMergesDuplicatesInBase(t *testing.T) {
	base := []byte(`
users:
  - id: alice
    role: user
    dept: eng
  - id: bob
    role: admin
  - id: alice
    role: manager
    team: platform
`)
	overlay := []byte(`
users:
  - id: alice
    active: true
`)

	result, err := mergeYAMLWith(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
		ObjectListMode:  keymerge.ObjectListConsolidate,
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Users []map[string]any `yaml:"users"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Should have 2 users: alice (consolidated) and bob
	if len(parsed.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(parsed.Users))
	}

	// First should be alice with merged fields
	alice := parsed.Users[0]
	if alice["id"] != "alice" {
		t.Fatalf("expected first user to be alice, got %v", alice["id"])
	}
	// Second alice should have merged into first, taking later values
	if alice["role"] != "manager" {
		t.Fatalf("expected role=manager (from second alice), got %v", alice["role"])
	}
	if alice["dept"] != "eng" {
		t.Fatalf("expected dept=eng (from first alice), got %v", alice["dept"])
	}
	if alice["team"] != "platform" {
		t.Fatalf("expected team=platform (from second alice), got %v", alice["team"])
	}
	if alice["active"] != true {
		t.Fatalf("expected active=true (from overlay), got %v", alice["active"])
	}

	// Second should be bob
	if parsed.Users[1]["id"] != "bob" {
		t.Fatalf("expected second user to be bob, got %v", parsed.Users[1]["id"])
	}
}

func TestObjectListMode_ConsolidateMergesDuplicatesInOverlay(t *testing.T) {
	base := []byte(`
users:
  - id: alice
    role: user
`)
	overlay := []byte(`
users:
  - id: alice
    dept: eng
  - id: bob
    role: admin
  - id: alice
    team: platform
`)

	result, err := mergeYAMLWith(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
		ObjectListMode:  keymerge.ObjectListConsolidate,
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Users []map[string]any `yaml:"users"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Should have 2 users
	if len(parsed.Users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(parsed.Users))
	}

	// Alice should have all fields merged
	alice := parsed.Users[0]
	if alice["id"] != "alice" {
		t.Fatalf("expected alice, got %v", alice["id"])
	}
	if alice["role"] != "user" {
		t.Fatalf("expected role=user, got %v", alice["role"])
	}
	if alice["dept"] != "eng" {
		t.Fatalf("expected dept=eng, got %v", alice["dept"])
	}
	if alice["team"] != "platform" {
		t.Fatalf("expected team=platform, got %v", alice["team"])
	}
}

func TestObjectListMode_UniqueIsDefault(t *testing.T) {
	base := []byte(`
users:
  - id: alice
    role: user
  - id: alice
    role: admin
`)
	overlay := []byte(`
users:
  - id: bob
    role: user
`)

	// Don't specify ObjectListMode, should default to Unique
	_, err := mergeYAMLWith(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
	}, base, overlay)

	if err == nil {
		t.Fatal("expected error (default should be Unique), got nil")
	}

	var dupErr *keymerge.DuplicatePrimaryKeyError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected DuplicatePrimaryKeyError, got %T", err)
	}

	// Path should be either users.0 or users.1 (the duplicate positions)
	if !slices.Equal(dupErr.Path, []string{"users", "0"}) && !slices.Equal(dupErr.Path, []string{"users", "1"}) {
		t.Fatalf("expected duplicate path 'users.0' or 'users.1', got %v", dupErr.Path)
	}
}

func TestNonComparablePrimaryKey_Map(t *testing.T) {
	base := map[string]any{
		"users": []any{
			map[string]any{
				"id":   map[string]any{"nested": "value"}, // Map as primary key - not comparable!
				"name": "alice",
			},
		},
	}
	overlay := map[string]any{
		"users": []any{
			map[string]any{
				"id":   map[string]any{"nested": "value"},
				"role": "admin",
			},
		},
	}

	_, err := keymerge.Merge(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
	}, base, overlay)

	if err == nil {
		t.Fatal("expected error for non-comparable primary key, got nil")
	}

	if !errors.Is(err, keymerge.ErrNonComparablePrimaryKey) {
		t.Errorf("expected errors.Is(err, ErrNonComparablePrimaryKey) to be true")
	}

	var ncErr *keymerge.NonComparablePrimaryKeyError
	if !errors.As(err, &ncErr) {
		t.Fatalf("expected NonComparablePrimaryKeyError, got %T: %v", err, err)
	}

	if ncErr.Position != 0 {
		t.Fatalf("expected position 0, got %d", ncErr.Position)
	}

	if !slices.Equal(ncErr.Path, []string{"users", "0"}) {
		t.Fatalf("expected non-comparable path 'users.0', got %v", ncErr.Path)
	}
}

func TestNonComparablePrimaryKey_Slice(t *testing.T) {
	base := map[string]any{
		"users": []any{
			map[string]any{
				"id":   []any{"foo", "bar"}, // Slice as primary key - not comparable!
				"name": "alice",
			},
		},
	}
	overlay := map[string]any{
		"users": []any{
			map[string]any{
				"id":   []any{"foo", "bar"},
				"role": "admin",
			},
		},
	}

	_, err := keymerge.Merge(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
		ObjectListMode:  keymerge.ObjectListConsolidate,
	}, base, overlay)

	if err == nil {
		t.Fatal("expected error for non-comparable primary key, got nil")
	}

	var ncErr *keymerge.NonComparablePrimaryKeyError
	if !errors.As(err, &ncErr) {
		t.Fatalf("expected NonComparablePrimaryKeyError, got %T: %v", err, err)
	}

	if !slices.Equal(ncErr.Path, []string{"users", "0"}) {
		t.Fatalf("expected non-comparable path 'users.0', got %v", ncErr.Path)
	}
}

func TestNonComparablePrimaryKey_InOverlay(t *testing.T) {
	base := []byte(`
users:
  - id: alice
    role: user
`)
	// YAML can't represent maps/slices as keys easily, so use direct data
	overlay := map[string]any{
		"users": []any{
			map[string]any{
				"id":   []any{"invalid"},
				"role": "admin",
			},
		},
	}

	baseData := make(map[string]any)
	if err := yaml.Unmarshal(base, &baseData); err != nil {
		t.Fatal(err)
	}

	_, err := keymerge.Merge(keymerge.Options{
		PrimaryKeyNames: []string{"id"},
	}, baseData, overlay)

	if err == nil {
		t.Fatal("expected error for non-comparable primary key in overlay, got nil")
	}

	var ncErr *keymerge.NonComparablePrimaryKeyError
	if !errors.As(err, &ncErr) {
		t.Fatalf("expected NonComparablePrimaryKeyError, got %T: %v", err, err)
	}

	if !slices.Equal(ncErr.Path, []string{"users", "0"}) {
		t.Fatalf("expected non-comparable path 'users.0', got %v", ncErr.Path)
	}
}

func TestPrimaryKeyDiscovery_SkipsItemsWithoutKeys(t *testing.T) {
	base := []byte(`
items:
  - name: item1
    value: 1
`)
	// First overlay item has no primary key, second one does
	overlay := []byte(`
items:
  - value: 999
  - name: item1
    value: 2
  - name: item2
    value: 3
`)

	result, err := mergeYAMLWith(keymerge.Options{
		PrimaryKeyNames: []string{"name"},
	}, base, overlay)
	if err != nil {
		t.Fatal(err)
	}

	var parsed struct {
		Items []map[string]any `yaml:"items"`
	}
	if err := yaml.Unmarshal(result, &parsed); err != nil {
		t.Fatal(err)
	}

	// Should have 3 items: item1 (merged with base), keyless item (appended), item2 (new)
	if len(parsed.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(parsed.Items))
	}

	// First should be item1 with updated value
	if parsed.Items[0]["name"] != "item1" || parsed.Items[0]["value"].(uint64) != 2 {
		t.Fatalf("expected item1 with value=2, got %v", parsed.Items[0])
	}

	// Second should be the keyless item
	if _, hasName := parsed.Items[1]["name"]; hasName {
		t.Fatalf("expected keyless item, got %v", parsed.Items[1])
	}
	if parsed.Items[1]["value"].(uint64) != 999 {
		t.Fatalf("expected keyless item with value=999, got %v", parsed.Items[1])
	}

	// Third should be item2
	if parsed.Items[2]["name"] != "item2" || parsed.Items[2]["value"].(uint64) != 3 {
		t.Fatalf("expected item2 with value=3, got %v", parsed.Items[2])
	}
}

func TestNestedArrayErrorPath(t *testing.T) {
	// Test that errors in nested arrays show complete paths
	base := map[string]any{
		"teams": []any{
			map[string]any{
				"name": "backend",
				"members": []any{
					map[string]any{"id": "alice", "role": "lead"},
					map[string]any{"id": "bob", "role": "dev"},
				},
			},
		},
	}

	overlay := map[string]any{
		"teams": []any{
			map[string]any{
				"name": "backend",
				"members": []any{
					map[string]any{"id": "alice", "role": "admin"},
					map[string]any{"id": map[string]any{"nested": "bad"}, "role": "dev"}, // Non-comparable!
				},
			},
		},
	}

	opts := keymerge.Options{
		PrimaryKeyNames: []string{"name", "id"},
	}

	_, err := keymerge.Merge(opts, base, overlay)
	if err == nil {
		t.Fatal("expected error for non-comparable primary key in nested array")
	}

	var ncErr *keymerge.NonComparablePrimaryKeyError
	if !errors.As(err, &ncErr) {
		t.Fatalf("expected NonComparablePrimaryKeyError, got %T: %v", err, err)
	}

	// Path should show the complete nested location: teams.0.members.1
	expectedPath := []string{"teams", "0", "members", "1"}
	if !slices.Equal(ncErr.Path, expectedPath) {
		t.Fatalf("expected path %v, got %v", expectedPath, ncErr.Path)
	}
}

func TestScalarListMode_String(t *testing.T) {
	tests := []struct {
		mode keymerge.ScalarListMode
		want string
	}{
		{keymerge.ScalarListConcat, "ScalarListConcat"},
		{keymerge.ScalarListDedup, "ScalarListDedup"},
		{keymerge.ScalarListReplace, "ScalarListReplace"},
		{keymerge.ScalarListMode(99), "ScalarListMode(99)"}, // Invalid value
	}

	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestObjectListMode_String(t *testing.T) {
	tests := []struct {
		mode keymerge.ObjectListMode
		want string
	}{
		{keymerge.ObjectListUnique, "ObjectListUnique"},
		{keymerge.ObjectListConsolidate, "ObjectListConsolidate"},
		{keymerge.ObjectListMode(99), "ObjectListMode(99)"}, // Invalid value
	}

	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}
