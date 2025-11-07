// SPDX-License-Identifier: Apache-2.0

// Package keymerge provides format-agnostic configuration merging with intelligent list handling.
//
// The library merges configuration documents by deep-merging maps and intelligently merging
// lists based on primary key fields. It works with any serialization format (YAML, JSON, TOML, etc.)
// that can unmarshal to map[string]any or []any.
package keymerge

import "fmt"

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

// ObjectListMode specifies how to handle duplicate primary keys in object lists.
type ObjectListMode int

const (
	// ObjectListUnique returns an error if duplicate primary keys are found (default behavior).
	ObjectListUnique ObjectListMode = iota
	// ObjectListConsolidate merges items with duplicate primary keys together.
	ObjectListConsolidate
)

// DuplicatePrimaryKeyError is returned when duplicate primary keys are found
// in a list and ObjectListMode is set to ObjectListUnique.
type DuplicatePrimaryKeyError struct {
	// Key is the duplicate primary key value
	Key any
	// Positions are the indices where the duplicate key was found
	Positions []int
}

func (e *DuplicatePrimaryKeyError) Error() string {
	return fmt.Sprintf("duplicate primary key %v found at positions %v", e.Key, e.Positions)
}

// NonComparablePrimaryKeyError is returned when a primary key value is not comparable
// (e.g., a map or slice). Primary key values must be comparable types (strings, numbers, bools, etc.).
type NonComparablePrimaryKeyError struct {
	// Key is the non-comparable primary key value
	Key any
	// Position is the index where the non-comparable key was found
	Position int
}

func (e *NonComparablePrimaryKeyError) Error() string {
	return fmt.Sprintf("non-comparable primary key %v (type %T) at position %d", e.Key, e.Key, e.Position)
}

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

	// ObjectListMode specifies how to handle items with duplicate primary keys in object lists.
	// Default is ObjectListUnique.
	ObjectListMode ObjectListMode
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
func Merge(opts Options, docs ...any) (any, error) {
	var result any
	var err error
	for _, doc := range docs {
		result, err = mergeValues(result, doc, opts)
		if err != nil {
			return nil, err
		}
	}

	// Strip delete marker keys from the final result
	if opts.DeleteMarkerKey != "" {
		result = stripDeleteMarker(result, opts.DeleteMarkerKey)
	}

	return result, nil
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
	result, err := Merge(opts, parsedDocs...)
	if err != nil {
		return nil, err
	}

	// Marshal back
	return marshal(result)
}

func mergeValues(base, overlay any, opts Options) (any, error) {
	// If overlay is nil, keep base
	if overlay == nil {
		return base, nil
	}

	// If base is nil, use overlay
	if base == nil {
		return overlay, nil
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
	return overlay, nil
}

func mergeMaps(base, overlay map[string]any, opts Options) (map[string]any, error) {
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
			merged, err := mergeValues(baseVal, v, opts)
			if err != nil {
				return nil, err
			}
			result[k] = merged
		} else {
			result[k] = v
		}
	}

	return result, nil
}

func mergeSlices(base, overlay []any, opts Options) ([]any, error) {
	// Check if items have primary keys
	if len(overlay) == 0 {
		return base, nil
	}

	// Try to find primary key in first overlay item
	primaryKey := findPrimaryKey(overlay[0], opts.PrimaryKeyNames)
	if primaryKey == "" {
		// No primary key, merge according to ScalarListMode
		switch opts.ScalarListMode {
		case ScalarListReplace:
			return overlay, nil
		case ScalarListDedup:
			return deduplicateList(base, overlay), nil
		default: // ScalarListConcat
			result := make([]any, len(base)+len(overlay))
			copy(result, base)
			copy(result[len(base):], overlay)
			return result, nil
		}
	}

	// Build index of items by primary key
	result := make([]any, 0, len(base))
	// resultIndex maps primary keys to positions in result.
	// Positions remain stable during merge because we mark deletions as nil
	// rather than removing items. Filtering happens only at the end.
	resultIndex := make(map[any]int, len(base))
	for i, item := range base {
		key := getPrimaryKeyValue(item, primaryKey)
		if key == nil {
			result = append(result, item)
			continue
		}

		// Check if key is comparable (can be used as map key)
		if !isComparable(key) {
			return nil, &NonComparablePrimaryKeyError{
				Key:      key,
				Position: i,
			}
		}

		existingIdx, exists := resultIndex[key]
		if !exists {
			resultIndex[key] = len(result)
			result = append(result, item)
			continue
		}

		// Duplicate found!
		if opts.ObjectListMode == ObjectListUnique {
			return nil, &DuplicatePrimaryKeyError{
				Key:       key,
				Positions: []int{existingIdx, i},
			}
		}

		// ObjectListConsolidate: merge into first occurrence
		merged, err := mergeValues(result[existingIdx], item, opts)
		if err != nil {
			return nil, err
		}
		result[existingIdx] = merged
		// Mark this duplicate for removal
		result = append(result, nil)
	}

	// Check for duplicates in overlay (if ObjectListUnique mode)
	if opts.ObjectListMode == ObjectListUnique {
		overlayKeys := make(map[any]int, len(overlay))
		for i, overlayItem := range overlay {
			if isMarkedForDeletion(overlayItem, opts.DeleteMarkerKey) {
				continue // Skip deletion markers
			}
			key := getPrimaryKeyValue(overlayItem, primaryKey)
			if key == nil {
				continue
			}

			// Check if key is comparable
			if !isComparable(key) {
				return nil, &NonComparablePrimaryKeyError{
					Key:      key,
					Position: i,
				}
			}

			if firstIdx, exists := overlayKeys[key]; exists {
				return nil, &DuplicatePrimaryKeyError{
					Key:       key,
					Positions: []int{firstIdx, i},
				}
			}
			overlayKeys[key] = i
		}
	}

	// Merge overlay items
	for i, overlayItem := range overlay {
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

		// Check if key is comparable (for Consolidate mode, Unique already checked)
		if opts.ObjectListMode != ObjectListUnique && !isComparable(key) {
			return nil, &NonComparablePrimaryKeyError{
				Key:      key,
				Position: i,
			}
		}

		if idx, exists := resultIndex[key]; exists {
			// Merge with existing item
			merged, err := mergeValues(result[idx], overlayItem, opts)
			if err != nil {
				return nil, err
			}
			result[idx] = merged
		} else {
			// Append new item
			result = append(result, overlayItem)
			resultIndex[key] = len(result) - 1
		}
	}

	// Filter out nil items (deleted items or consolidated duplicates)
	if opts.DeleteMarkerKey != "" || opts.ObjectListMode == ObjectListConsolidate {
		filtered := make([]any, 0, len(result))
		for _, item := range result {
			if item != nil {
				filtered = append(filtered, item)
			}
		}
		return filtered, nil
	}

	return result, nil
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

// isComparable checks if a value is comparable (can be used as a map key).
// Maps and slices are not comparable in Go.
func isComparable(value any) bool {
	if value == nil {
		return true
	}
	switch value.(type) {
	case map[string]any, []any:
		return false
	default:
		return true
	}
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
	seen := make(map[any]struct{}, len(base)+len(overlay))

	// Add items from base
	for _, item := range base {
		switch item.(type) {
		case map[string]any, []any:
			// Maps and slices aren't comparable, always add them
			result = append(result, item)
		default:
			// For scalars, use map to track uniqueness
			if _, exists := seen[item]; !exists {
				seen[item] = struct{}{}
				result = append(result, item)
			}
		}
	}

	// Add items from overlay
	for _, item := range overlay {
		switch item.(type) {
		case map[string]any, []any:
			// Maps and slices aren't comparable, always add them
			result = append(result, item)
		default:
			// For scalars, use map to track uniqueness
			if _, exists := seen[item]; !exists {
				seen[item] = struct{}{}
				result = append(result, item)
			}
		}
	}

	return result
}
