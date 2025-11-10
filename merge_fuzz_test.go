// SPDX-License-Identifier: Apache-2.0

package keymerge_test

import (
	"testing"

	"github.com/sam-fredrickson/keymerge"
)

// FuzzMergeComplexStructures fuzzes the MergeUnstructured function with complex nested structures.
// This tests the core merge algorithm with maps, slices, and various data types.
func FuzzMergeComplexStructures(f *testing.F) {
	// Seed with interesting values that will be used to build structures
	f.Add(int64(1), int64(2), "alice", "bob")
	f.Add(int64(0), int64(-1), "x", "y")
	f.Add(int64(100), int64(200), "name", "id")

	f.Fuzz(func(t *testing.T, num1, num2 int64, str1, str2 string) {
		// Build complex nested structures
		base := map[string]any{
			"scalar":    num1,
			"string":    str1,
			"list":      []any{num1, str1, num1 + 1},
			"nested":    map[string]any{"deep": map[string]any{"value": num1}},
			"mixed":     []any{map[string]any{"key": str1}, num1},
			"users":     []any{map[string]any{"name": str1, "age": num1}},
			"nullValue": nil,
		}
		overlay := map[string]any{
			"scalar":    num2,
			"string":    str2,
			"list":      []any{num2, str2},
			"nested":    map[string]any{"deep": map[string]any{"other": num2}},
			"mixed":     []any{map[string]any{"key": str2}, num2},
			"users":     []any{map[string]any{"name": str2, "age": num2}},
			"newField":  "added",
			"nullValue": num2,
		}

		opts := keymerge.Options{
			PrimaryKeyNames: []string{"name", "id"},
			ScalarMode:      keymerge.ScalarConcat,
		}

		// Should not panic
		result, err := keymerge.MergeUnstructured(opts, base, overlay)
		if err != nil {
			t.Skip("merge failed (expected for some inputs)")
		}

		// Result should be a map
		if result == nil {
			t.Fatal("result is nil")
		}
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("result is not a map: %T", result)
		}

		// Verify some basic properties
		if _, hasNewField := resultMap["newField"]; !hasNewField {
			t.Fatal("overlay fields should be present in result")
		}

		// Verify the merge was performed (result should have both values in some form)
		if resultMap["scalar"] == nil {
			t.Fatal("scalar field should not be nil after merge")
		}
	})
}

// FuzzMergeDirect fuzzes the MergeUnstructured function with already-unmarshaled data.
// This tests the core merge logic without YAML parsing complications.
func FuzzMergeDirect(f *testing.F) {
	// Seed with some basic structures
	f.Add(int64(1), int64(2))
	f.Add(int64(0), int64(0))
	f.Add(int64(-1), int64(1))

	f.Fuzz(func(t *testing.T, a, b int64) {
		// Build simple test structures
		base := map[string]any{
			"value":  a,
			"items":  []any{a, a + 1},
			"nested": map[string]any{"x": a},
		}
		overlay := map[string]any{
			"value":  b,
			"items":  []any{b, b + 1},
			"nested": map[string]any{"y": b},
		}

		opts := keymerge.Options{
			PrimaryKeyNames: []string{"id"},
			ScalarMode:      keymerge.ScalarDedup,
		}

		// Should not panic
		result, err := keymerge.MergeUnstructured(opts, base, overlay)
		if err != nil {
			t.Skip("merge failed (expected for some inputs)")
		}

		// Result should be a map
		if result == nil {
			t.Fatal("result is nil")
		}
		if _, ok := result.(map[string]any); !ok {
			t.Fatalf("result is not a map: %T", result)
		}
	})
}

// FuzzMergeWithPrimaryKeys fuzzes merging lists with primary keys.
func FuzzMergeWithPrimaryKeys(f *testing.F) {
	// Seed with some interesting cases
	f.Add(int64(1), int64(1)) // Same ID
	f.Add(int64(1), int64(2)) // Different IDs
	f.Add(int64(0), int64(-1))

	f.Fuzz(func(t *testing.T, id1, id2 int64) {
		base := map[string]any{
			"users": []any{
				map[string]any{"id": id1, "name": "user1"},
			},
		}
		overlay := map[string]any{
			"users": []any{
				map[string]any{"id": id2, "name": "user2"},
			},
		}

		opts := keymerge.Options{
			PrimaryKeyNames: []string{"id"},
		}

		// Should not panic
		result, err := keymerge.MergeUnstructured(opts, base, overlay)
		if err != nil {
			// Errors are OK (e.g., duplicate keys in Unique mode)
			return
		}

		// Verify result structure
		resultMap, ok := result.(map[string]any)
		if !ok {
			t.Fatalf("result is not a map: %T", result)
		}

		users, ok := resultMap["users"].([]any)
		if !ok {
			t.Fatalf("users is not a slice: %T", resultMap["users"])
		}

		// Should have 1 or 2 users depending on whether IDs matched
		if id1 == id2 {
			if len(users) != 1 {
				t.Fatalf("expected 1 user for matching IDs, got %d", len(users))
			}
		} else {
			if len(users) != 2 {
				t.Fatalf("expected 2 users for different IDs, got %d", len(users))
			}
		}
	})
}

// FuzzMergeScalarModes tests different scalar list modes.
func FuzzMergeScalarModes(f *testing.F) {
	// Seed with various list configurations
	f.Add(int64(1), int64(2), int64(3))
	f.Add(int64(0), int64(0), int64(0))
	f.Add(int64(-1), int64(0), int64(1))

	f.Fuzz(func(t *testing.T, a, b, c int64) {
		base := map[string]any{
			"tags": []any{a, b},
		}
		overlay := map[string]any{
			"tags": []any{b, c},
		}

		modes := []keymerge.ScalarMode{
			keymerge.ScalarConcat,
			keymerge.ScalarDedup,
			keymerge.ScalarReplace,
		}

		for _, mode := range modes {
			opts := keymerge.Options{
				ScalarMode: mode,
			}

			result, err := keymerge.MergeUnstructured(opts, base, overlay)
			if err != nil {
				t.Fatalf("merge failed with mode %d: %v", mode, err)
			}

			resultMap := result.(map[string]any)
			tags := resultMap["tags"].([]any)

			// Verify expected behavior
			switch mode {
			case keymerge.ScalarConcat:
				if len(tags) != 4 {
					t.Fatalf("concat mode: expected 4 items, got %d", len(tags))
				}
			case keymerge.ScalarReplace:
				if len(tags) != 2 {
					t.Fatalf("replace mode: expected 2 items, got %d", len(tags))
				}
			case keymerge.ScalarDedup:
				// Dedup length depends on uniqueness (could be 1 if all values same)
				if len(tags) < 1 || len(tags) > 4 {
					t.Fatalf("dedup mode: expected 1-4 items, got %d", len(tags))
				}
			}
		}
	})
}
