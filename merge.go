// SPDX-License-Identifier: Apache-2.0

// Package keymerge provides format-agnostic configuration merging with intelligent list handling.
//
// The library merges configuration documents by deep-merging maps and intelligently merging
// lists based on primary key fields. It works with any serialization format (YAML, JSON, TOML, etc.)
// that can unmarshal to map[string]any or []any.
package keymerge

// ScalarListMode specifies how to merge lists that don't have primary keys.
type ScalarListMode int

const (
	// ScalarListConcat appends overlay list items to base list items (default behavior).
	ScalarListConcat ScalarListMode = iota
	// ScalarListDedup concatenates lists and removes duplicate values.
	ScalarListDedup
	// ScalarListReplace replaces the base list entirely with the overlay list.
	ScalarListReplace
)

// Options configures the merge behavior.
type Options struct {
	// PrimaryKeyNames specifies field names to use as primary keys when merging lists.
	// When merging lists of maps, the first matching field name is used to identify
	// corresponding items across documents. Items with matching keys are deep-merged;
	// items without matches are appended.
	//
	// For example, with PrimaryKeyNames ["name", "id"], a list item will be matched
	// by "name" if present, otherwise by "id" if present. If neither field exists,
	// the item is treated as having no key and will be concatenated according to ScalarListMode.
	//
	// Lists of non-map items (e.g., []string) are merged according to ScalarListMode.
	PrimaryKeyNames []string

	// DeleteMarkerKey specifies a field name that marks items for deletion.
	// When set, if a map contains this field with a value of true, the item is removed
	// from the result instead of being merged.
	//
	// For map values: setting {key: {DeleteMarkerKey: true}} removes that key.
	// For list items: setting {primaryKey: "foo", DeleteMarkerKey: true} removes the item with that key.
	//
	// If empty, deletion semantics are disabled.
	DeleteMarkerKey string

	// ScalarListMode specifies how to merge lists without primary keys.
	// Default is ScalarListConcat.
	ScalarListMode ScalarListMode
}

// Merge merges multiple documents together.
//
// Documents are merged left-to-right, with later documents taking precedence.
// Maps are deep-merged recursively. Lists are merged by primary key if items are maps
// containing a primary key field; otherwise lists are concatenated. Scalar values
// are replaced by later values.
//
// This function is format-agnostic and works with any unmarshalled data structure.
// The input documents should be map[string]any, []any, or scalar values.
//
// Example:
//
//	opts := Options{PrimaryKeyNames: []string{"id", "name"}}
//	base := map[string]any{"users": []any{
//		map[string]any{"name": "alice", "role": "user"},
//	}}
//	overlay := map[string]any{"users": []any{
//		map[string]any{"name": "alice", "role": "admin"},
//	}}
//	result := Merge(opts, base, overlay)
//	// Result: alice's role is updated to "admin"
func Merge(opts Options, docs ...any) any {
	var result any
	for _, doc := range docs {
		result = mergeValues(result, doc, opts)
	}

	// Strip delete marker keys from the final result
	if opts.DeleteMarkerKey != "" {
		result = stripDeleteMarker(result, opts.DeleteMarkerKey)
	}

	return result
}

// MergeMarshal merges multiple byte documents using provided marshaling functions.
//
// This function handles unmarshaling documents, merging them with Merge, and marshaling
// the result back to bytes. It allows the merge algorithm to work with any serialization
// format (YAML, JSON, TOML, etc.) by accepting custom unmarshal and marshal functions.
//
// Documents are merged left-to-right. If docs is empty, returns an empty byte slice.
// Returns an error if unmarshaling or marshaling fails.
//
// Example with YAML:
//
//	import "github.com/goccy/go-yaml"
//
//	opts := Options{PrimaryKeyNames: []string{"name"}}
//	base := []byte("users:\n  - name: alice\n    role: user")
//	overlay := []byte("users:\n  - name: alice\n    role: admin")
//	result, err := MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, base, overlay)
//
// Example with JSON:
//
//	import "encoding/json"
//
//	opts := Options{PrimaryKeyNames: []string{"id"}}
//	result, err := MergeMarshal(opts, json.Unmarshal, json.Marshal, base, overlay)
func MergeMarshal(
	opts Options,
	unmarshal func([]byte, any) error,
	marshal func(any) ([]byte, error),
	docs ...[]byte,
) ([]byte, error) {
	if len(docs) == 0 {
		return []byte{}, nil
	}

	// Parse all documents
	parsedDocs := make([]any, len(docs))
	for i, doc := range docs {
		var current any
		if err := unmarshal(doc, &current); err != nil {
			return nil, err
		}
		parsedDocs[i] = current
	}

	// Merge
	result := Merge(opts, parsedDocs...)

	// Marshal back
	return marshal(result)
}

func mergeValues(base, overlay any, opts Options) any {
	// If overlay is nil, keep base
	if overlay == nil {
		return base
	}

	// If base is nil, use overlay
	if base == nil {
		return overlay
	}

	// Handle maps
	baseMap, baseIsMap := base.(map[string]any)
	overlayMap, overlayIsMap := overlay.(map[string]any)
	if baseIsMap && overlayIsMap {
		return mergeMaps(baseMap, overlayMap, opts)
	}

	// Handle slices
	baseSlice, baseIsSlice := base.([]any)
	overlaySlice, overlayIsSlice := overlay.([]any)
	if baseIsSlice && overlayIsSlice {
		return mergeSlices(baseSlice, overlaySlice, opts)
	}

	// For scalar values, overlay wins
	return overlay
}

func mergeMaps(base, overlay map[string]any, opts Options) map[string]any {
	result := make(map[string]any)

	// Copy base
	for k, v := range base {
		result[k] = v
	}

	// Merge overlay
	for k, v := range overlay {
		// Check if this key is marked for deletion
		if isMarkedForDeletion(v, opts.DeleteMarkerKey) {
			delete(result, k)
			continue
		}

		if baseVal, exists := result[k]; exists {
			result[k] = mergeValues(baseVal, v, opts)
		} else {
			result[k] = v
		}
	}

	return result
}

func mergeSlices(base, overlay []any, opts Options) []any {
	// Check if items have primary keys
	if len(overlay) == 0 {
		return base
	}

	// Try to find primary key in first overlay item
	primaryKey := findPrimaryKey(overlay[0], opts.PrimaryKeyNames)
	if primaryKey == "" {
		// No primary key, merge according to ScalarListMode
		switch opts.ScalarListMode {
		case ScalarListReplace:
			return overlay
		case ScalarListDedup:
			return deduplicateList(base, overlay)
		default: // ScalarListConcat
			result := make([]any, len(base)+len(overlay))
			copy(result, base)
			copy(result[len(base):], overlay)
			return result
		}
	}

	// Build index of items by primary key
	result := make([]any, 0, len(base))
	// resultIndex maps primary keys to positions in result.
	// Positions remain stable during merge because we mark deletions as nil
	// rather than removing items. Filtering happens only at the end.
	resultIndex := make(map[any]int, len(base))
	for i, item := range base {
		if key := getPrimaryKeyValue(item, primaryKey); key != nil {
			resultIndex[key] = i
		}
		result = append(result, item)
	}

	// Merge overlay items
	for _, overlayItem := range overlay {
		// Check if this item is marked for deletion
		if isMarkedForDeletion(overlayItem, opts.DeleteMarkerKey) {
			key := getPrimaryKeyValue(overlayItem, primaryKey)
			if key != nil {
				if idx, exists := resultIndex[key]; exists {
					// Mark for deletion by setting to nil, we'll filter later
					result[idx] = nil
					delete(resultIndex, key)
				}
			}
			continue
		}

		key := getPrimaryKeyValue(overlayItem, primaryKey)
		if key == nil {
			// No key, append
			result = append(result, overlayItem)
			continue
		}

		if idx, exists := resultIndex[key]; exists {
			// Merge with existing item
			result[idx] = mergeValues(result[idx], overlayItem, opts)
		} else {
			// Append new item
			result = append(result, overlayItem)
			resultIndex[key] = len(result) - 1
		}
	}

	// Filter out nil items (deleted items)
	if opts.DeleteMarkerKey != "" {
		filtered := make([]any, 0, len(result))
		for _, item := range result {
			if item != nil {
				filtered = append(filtered, item)
			}
		}
		return filtered
	}

	return result
}

// stripDeleteMarker removes the delete marker key from a value recursively.
func stripDeleteMarker(value any, deleteMarkerKey string) any {
	switch v := value.(type) {
	case map[string]any:
		// Create new map without the delete marker
		result := make(map[string]any, len(v))
		for k, val := range v {
			if k != deleteMarkerKey {
				result[k] = stripDeleteMarker(val, deleteMarkerKey)
			}
		}
		return result
	case []any:
		// Recursively strip from list items
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = stripDeleteMarker(item, deleteMarkerKey)
		}
		return result
	default:
		return value
	}
}

func findPrimaryKey(item any, keyNames []string) string {
	m, ok := item.(map[string]any)
	if !ok {
		return ""
	}

	for _, keyName := range keyNames {
		if _, exists := m[keyName]; exists {
			return keyName
		}
	}

	return ""
}

func getPrimaryKeyValue(item any, keyName string) any {
	m, ok := item.(map[string]any)
	if !ok {
		return nil
	}

	return m[keyName]
}

// isMarkedForDeletion checks if a value has the delete marker set to true.
func isMarkedForDeletion(value any, deleteMarkerKey string) bool {
	if deleteMarkerKey == "" {
		return false
	}

	m, ok := value.(map[string]any)
	if !ok {
		return false
	}

	marker, exists := m[deleteMarkerKey]
	if !exists {
		return false
	}

	// Check if marker is true (handle bool type)
	if b, ok := marker.(bool); ok {
		return b
	}

	return false
}

// deduplicateList concatenates base and overlay, removing duplicate values.
// For scalar values (strings, numbers, bools), uses exact equality.
// For maps and slices, no deduplication is performed (they're always considered unique).
func deduplicateList(base, overlay []any) []any {
	result := make([]any, 0, len(base)+len(overlay))

	// Helper to check if item already exists in result
	contains := func(list []any, item any) bool {
		// Only deduplicate comparable types
		switch item.(type) {
		case map[string]any, []any:
			// Maps and slices aren't comparable, always add them
			return false
		default:
			// For scalars, check for exact match
			for _, existing := range list {
				if existing == item {
					return true
				}
			}
			return false
		}
	}

	// Add items from base
	for _, item := range base {
		if !contains(result, item) {
			result = append(result, item)
		}
	}

	// Add items from overlay
	for _, item := range overlay {
		if !contains(result, item) {
			result = append(result, item)
		}
	}

	return result
}
