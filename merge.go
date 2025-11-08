// SPDX-License-Identifier: Apache-2.0

// Package keymerge provides format-agnostic configuration merging with intelligent list handling.
//
// The library deep-merges maps and intelligently merges lists by matching items on primary key fields.
// It works with any serialization format (YAML, JSON, TOML, etc.) that unmarshals to map[string]any or []any.
package keymerge

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// Sentinel errors for simple error checking with [errors.Is].
// For detailed error information, use [errors.As] with the typed errors below.
var (
	// ErrDuplicatePrimaryKey indicates duplicate primary keys were found in a list.
	ErrDuplicatePrimaryKey = errors.New("duplicate primary key")
	// ErrNonComparablePrimaryKey indicates a primary key value is not comparable (e.g., a map or slice).
	ErrNonComparablePrimaryKey = errors.New("non-comparable primary key")
	// ErrMarshal indicates a marshaling or unmarshaling operation failed.
	ErrMarshal = errors.New("marshal error")
	// ErrInvalidOptions indicates invalid merge options were provided.
	ErrInvalidOptions = errors.New("invalid options")
)

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

func (m ScalarListMode) String() string {
	switch m {
	case ScalarListConcat:
		return "ScalarListConcat"
	case ScalarListDedup:
		return "ScalarListDedup"
	case ScalarListReplace:
		return "ScalarListReplace"
	default:
		return fmt.Sprintf("ScalarListMode(%d)", m)
	}
}

// ObjectListMode specifies how to handle duplicate primary keys in object lists.
type ObjectListMode int

const (
	// ObjectListUnique returns an error if duplicate primary keys are found (default behavior).
	ObjectListUnique ObjectListMode = iota
	// ObjectListConsolidate merges items with duplicate primary keys together.
	ObjectListConsolidate
)

func (m ObjectListMode) String() string {
	switch m {
	case ObjectListUnique:
		return "ObjectListUnique"
	case ObjectListConsolidate:
		return "ObjectListConsolidate"
	default:
		return fmt.Sprintf("ObjectListMode(%d)", m)
	}
}

// DuplicatePrimaryKeyError is returned when duplicate primary keys are found
// in a list and [ObjectListMode] is set to [ObjectListUnique].
type DuplicatePrimaryKeyError struct {
	// Key is the duplicate primary key value
	Key any
	// Positions are the indices where the duplicate key was found
	Positions []int
	// Path is where in the document the duplicate primary key value occurred.
	Path []string
	// DocIndex tells which document the error occurred.
	DocIndex int
}

func (e *DuplicatePrimaryKeyError) Error() string {
	path := strings.Join(e.Path, ".")
	if path == "" {
		path = "(root)"
	}
	return fmt.Sprintf("duplicate primary key %v at path %s in document %d at positions %v",
		e.Key, path, e.DocIndex, e.Positions)
}

func (e *DuplicatePrimaryKeyError) Is(target error) bool {
	return target == ErrDuplicatePrimaryKey
}

// NonComparablePrimaryKeyError is returned when a primary key value is not comparable
// (e.g., a map or slice). Primary key values must be comparable types (strings, numbers, bools, etc.).
type NonComparablePrimaryKeyError struct {
	// Key is the non-comparable primary key value
	Key any
	// Position is the index where the non-comparable key was found
	Position int
	// Path is where in the document the duplicate primary key value occurred.
	Path []string
	// DocIndex tells which document the error occurred.
	DocIndex int
}

func (e *NonComparablePrimaryKeyError) Error() string {
	path := strings.Join(e.Path, ".")
	if path == "" {
		path = "(root)"
	}
	return fmt.Sprintf("non-comparable primary key %v (type %T) at path %s in document %d at position %d",
		e.Key, e.Key, path, e.DocIndex, e.Position)
}

func (e *NonComparablePrimaryKeyError) Is(target error) bool {
	return target == ErrNonComparablePrimaryKey
}

// MarshalError is returned when unmarshaling or marshaling a document fails.
type MarshalError struct {
	// Err is the underlying error returned by a marshaling function.
	Err error
	// DocIndex tells which document the error occurred.
	DocIndex int
}

func (e *MarshalError) Error() string {
	return fmt.Sprintf("cannot marshal document at position %d: %v", e.DocIndex, e.Err)
}

func (e *MarshalError) Unwrap() error {
	return e.Err
}

func (e *MarshalError) Is(target error) bool {
	return target == ErrMarshal
}

// Options configures merge behavior.
//
// The zero value is valid and provides sensible defaults:
//   - No primary key matching (all lists treated as scalar lists)
//   - [ScalarListConcat] mode (lists are concatenated)
//   - No deletion markers
//   - [ObjectListUnique] mode (errors on duplicates, though none detected without primary keys)
type Options struct {
	// PrimaryKeyNames specifies field names to use as primary keys when merging lists.
	// The first matching field name identifies corresponding items across documents.
	// Items with matching keys are deep-merged; items without matches are appended.
	//
	// Example: ["name", "id"] tries "name" first, then "id". Items without either field
	// are treated as having no key and merged according to [ScalarListMode].
	PrimaryKeyNames []string

	// DeleteMarkerKey specifies a field name that marks items for deletion.
	// When set, maps with this field set to true are removed from the result.
	// If empty, deletion semantics are disabled.
	DeleteMarkerKey string

	// ScalarListMode specifies how to merge lists without primary keys.
	// Default is [ScalarListConcat].
	ScalarListMode ScalarListMode

	// ObjectListMode specifies how to handle duplicate primary keys in object lists.
	// Default is [ObjectListUnique].
	ObjectListMode ObjectListMode
}

// Merger performs document merging with the configured options.
// It tracks the current document path for detailed error reporting.
//
// A Merger can be safely reused for multiple merge operations.
//
// A Merger is not safe to use concurrently.
type Merger struct {
	opts  Options  // merge configuration
	path  []string // current path in document tree for error reporting
	index int      // current document index being processed
}

// NewMerger creates a new [Merger] with the given options.
// Returns an error if the options are invalid.
func NewMerger(opts Options) (*Merger, error) {
	for _, name := range opts.PrimaryKeyNames {
		if name == "" {
			return nil, fmt.Errorf("%w: empty string in PrimaryKeyNames", ErrInvalidOptions)
		}
	}
	return &Merger{opts: opts}, nil
}

// Options returns the merge options configured for this [Merger].
func (m *Merger) Options() Options {
	return m.opts
}

// Merge merges multiple documents. See [Merger.Merge] for details.
func Merge(opts Options, docs ...any) (any, error) {
	m, err := NewMerger(opts)
	if err != nil {
		return nil, err
	}
	return m.Merge(docs...)
}

// MergeMarshal merges byte documents using provided unmarshal and marshal functions.
// See [Merger.MergeMarshal] for details.
func MergeMarshal(
	opts Options,
	unmarshal func([]byte, any) error,
	marshal func(any) ([]byte, error),
	docs ...[]byte,
) ([]byte, error) {
	m, err := NewMerger(opts)
	if err != nil {
		return nil, err
	}
	return m.MergeMarshal(unmarshal, marshal, docs...)
}

// Merge merges multiple documents left-to-right, with later documents taking precedence.
//
// Maps are deep-merged recursively. Lists are merged by primary key if items contain
// a primary key field; otherwise merged according to [ScalarListMode]. Scalar values
// are replaced by later values.
//
// Duplicate items in lists are handled according to [ObjectListMode].
//
// Input documents should be map[string]any, []any, or scalar values.
//
// Example:
//
//	opts := Options{PrimaryKeyNames: []string{"name"}}
//	base := map[string]any{"users": []any{
//		map[string]any{"name": "alice", "role": "user"},
//	}}
//	overlay := map[string]any{"users": []any{
//		map[string]any{"name": "alice", "role": "admin"},
//	}}
//	result, _ := Merge(opts, base, overlay)
//	// Result: alice's role updated to "admin"
func (m *Merger) Merge(docs ...any) (any, error) {
	var result any
	var err error
	for i, doc := range docs {
		m.reset(i)
		result, err = m.mergeValues(result, doc)
		if err != nil {
			return nil, err
		}
	}

	// Strip delete marker keys from the final result
	result = m.stripDeleteMarker(result)

	return result, nil
}

// MergeMarshal merges byte documents using provided unmarshal and marshal functions.
//
// Documents are unmarshaled, merged left-to-right with [Merger.Merge], then marshaled back to bytes.
// Works with any serialization format (YAML, JSON, TOML, etc.) via custom marshal functions.
//
// Returns an empty byte slice if docs is empty. Returns an error if unmarshaling,
// merging, or marshaling fails.
//
// Example:
//
//	import "github.com/goccy/go-yaml"
//
//	opts := Options{PrimaryKeyNames: []string{"name"}}
//	base := []byte("users:\n  - name: alice\n    role: user")
//	overlay := []byte("users:\n  - name: alice\n    role: admin")
//	result, _ := MergeMarshal(opts, yaml.Unmarshal, yaml.Marshal, base, overlay)
func (m *Merger) MergeMarshal(
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
			return nil, &MarshalError{
				Err:      err,
				DocIndex: i,
			}
		}
		parsedDocs[i] = current
	}

	// Merge
	result, err := m.Merge(parsedDocs...)
	if err != nil {
		return nil, err
	}

	// Marshal back
	return marshal(result)
}

func (m *Merger) reset(i int) {
	m.path = nil
	m.index = i
}

func (m *Merger) push(path string) {
	m.path = append(m.path, path)
}

func (m *Merger) pop() {
	if len(m.path) == 0 {
		panic("unbalanced keymerge.Merger pop")
	}
	m.path = m.path[:len(m.path)-1]
}

func (m *Merger) mergeValues(base, overlay any) (any, error) {
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
		return m.mergeMaps(baseMap, overlayMap)
	}

	// Handle slices
	baseSlice, baseIsSlice := base.([]any)
	overlaySlice, overlayIsSlice := overlay.([]any)
	if baseIsSlice && overlayIsSlice {
		return m.mergeSlices(baseSlice, overlaySlice)
	}

	// For scalar values, overlay wins
	return overlay, nil
}

func (m *Merger) mergeMaps(base, overlay map[string]any) (map[string]any, error) {
	// Pre-allocate for base size since overlay keys may overlap
	result := make(map[string]any, len(base))

	// Copy base
	for k, v := range base {
		result[k] = v
	}

	// Merge overlay
	for k, v := range overlay {
		m.push(k)

		// Check if this key is marked for deletion
		if m.isMarkedForDeletion(v) {
			delete(result, k)
			continue
		}

		if baseVal, exists := result[k]; exists {
			merged, err := m.mergeValues(baseVal, v)
			if err != nil {
				return nil, err
			}
			result[k] = merged
		} else {
			result[k] = v
		}

		m.pop()
	}

	return result, nil
}

func (m *Merger) mergeSlices(base, overlay []any) ([]any, error) {
	// Check if items have primary keys
	if len(overlay) == 0 {
		return base, nil
	}

	// Try to find primary key by checking overlay items until we find one.
	// This handles cases where the first item might not have a primary key
	// but subsequent items do.
	primaryKey := ""
	for _, item := range overlay {
		primaryKey = m.findPrimaryKey(item)
		if primaryKey != "" {
			break
		}
	}

	if primaryKey == "" {
		// No primary key found in any overlay item, merge according to ScalarListMode
		switch m.opts.ScalarListMode {
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
		m.push(fmt.Sprintf("%d", i))

		key := getPrimaryKeyValue(item, primaryKey)
		if key == nil {
			result = append(result, item)
			m.pop()
			continue
		}

		// Check if key is comparable (can be used as map key)
		if !isComparable(key) {
			err := &NonComparablePrimaryKeyError{
				Key:      key,
				Position: i,
				Path:     slices.Clone(m.path),
				DocIndex: m.index,
			}
			m.pop()
			return nil, err
		}

		existingIdx, exists := resultIndex[key]
		if !exists {
			resultIndex[key] = len(result)
			result = append(result, item)
			m.pop()
			continue
		}

		// Duplicate found!
		if m.opts.ObjectListMode == ObjectListUnique {
			err := &DuplicatePrimaryKeyError{
				Key:       key,
				Positions: []int{existingIdx, i},
				Path:      slices.Clone(m.path),
				DocIndex:  m.index,
			}
			m.pop()
			return nil, err
		}

		// ObjectListConsolidate: merge into first occurrence
		m.pop()                                // Pop current index before merging
		m.push(fmt.Sprintf("%d", existingIdx)) // Push existing index for merge
		merged, err := m.mergeValues(result[existingIdx], item)
		m.pop()
		if err != nil {
			return nil, err
		}
		result[existingIdx] = merged
	}

	// Check for duplicates in overlay (if ObjectListUnique mode)
	if m.opts.ObjectListMode == ObjectListUnique {
		overlayKeys := make(map[any]int, len(overlay))
		for i, overlayItem := range overlay {
			m.push(fmt.Sprintf("%d", i))

			if m.isMarkedForDeletion(overlayItem) {
				m.pop()
				continue // Skip deletion markers
			}
			key := getPrimaryKeyValue(overlayItem, primaryKey)
			if key == nil {
				m.pop()
				continue
			}

			// Check if key is comparable
			if !isComparable(key) {
				err := &NonComparablePrimaryKeyError{
					Key:      key,
					Position: i,
					Path:     slices.Clone(m.path),
					DocIndex: m.index,
				}
				m.pop()
				return nil, err
			}

			if firstIdx, exists := overlayKeys[key]; exists {
				err := &DuplicatePrimaryKeyError{
					Key:       key,
					Positions: []int{firstIdx, i},
					Path:      slices.Clone(m.path),
					DocIndex:  m.index,
				}
				m.pop()
				return nil, err
			}
			overlayKeys[key] = i
			m.pop()
		}
	}

	// Merge overlay items
	for i, overlayItem := range overlay {
		m.push(fmt.Sprintf("%d", i))

		// Check if this item is marked for deletion
		if m.isMarkedForDeletion(overlayItem) {
			key := getPrimaryKeyValue(overlayItem, primaryKey)
			if key != nil {
				if idx, exists := resultIndex[key]; exists {
					// Mark for deletion by setting to nil, we'll filter later
					result[idx] = nil
					delete(resultIndex, key)
				}
			}
			m.pop()
			continue
		}

		key := getPrimaryKeyValue(overlayItem, primaryKey)
		if key == nil {
			// No key, append
			result = append(result, overlayItem)
			m.pop()
			continue
		}

		// Check if key is comparable (for Consolidate mode, Unique already checked)
		if m.opts.ObjectListMode != ObjectListUnique && !isComparable(key) {
			err := &NonComparablePrimaryKeyError{
				Key:      key,
				Position: i,
				Path:     slices.Clone(m.path),
				DocIndex: m.index,
			}
			m.pop()
			return nil, err
		}

		if idx, exists := resultIndex[key]; exists {
			// Merge with existing item
			m.pop()                        // Pop current index before merging
			m.push(fmt.Sprintf("%d", idx)) // Push existing index for merge
			merged, err := m.mergeValues(result[idx], overlayItem)
			m.pop()
			if err != nil {
				return nil, err
			}
			result[idx] = merged
		} else {
			// Append new item
			result = append(result, overlayItem)
			resultIndex[key] = len(result) - 1
			m.pop()
		}
	}

	// Filter out nil items (deleted items or consolidated duplicates)
	if m.opts.DeleteMarkerKey != "" || m.opts.ObjectListMode == ObjectListConsolidate {
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
func (m *Merger) stripDeleteMarker(value any) any {
	if m.opts.DeleteMarkerKey == "" {
		return value
	}
	switch v := value.(type) {
	case map[string]any:
		// Create new map without the delete marker
		result := make(map[string]any, len(v))
		for k, val := range v {
			if k != m.opts.DeleteMarkerKey {
				result[k] = m.stripDeleteMarker(val)
			}
		}
		return result
	case []any:
		// Recursively strip from list items
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = m.stripDeleteMarker(item)
		}
		return result
	default:
		return value
	}
}

// findPrimaryKey returns the first primary key field name found in item.
// Returns empty string if item is not a map or no primary key field exists.
//
// Note: An empty string "" is technically a valid field name in Go maps,
// but using it as a primary key name is not recommended as it cannot be
// distinguished from the "not found" return value.
func (m *Merger) findPrimaryKey(item any) string {
	mp, ok := item.(map[string]any)
	if !ok {
		return ""
	}

	for _, keyName := range m.opts.PrimaryKeyNames {
		if _, exists := mp[keyName]; exists {
			return keyName
		}
	}

	return ""
}

// getPrimaryKeyValue returns the value of the primary key field.
// Returns nil if:
//   - item is not a map
//   - the key doesn't exist in the map
//   - the key exists but has an explicit nil/null value (treated same as missing)
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
	return reflect.TypeOf(value).Comparable()
}

// isMarkedForDeletion checks if a value has the delete marker set to true.
func (m *Merger) isMarkedForDeletion(value any) bool {
	if m.opts.DeleteMarkerKey == "" {
		return false
	}

	mp, ok := value.(map[string]any)
	if !ok {
		return false
	}

	marker, exists := mp[m.opts.DeleteMarkerKey]
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
// For maps and slices, no deduplication is performed (they're always considered unique)
// because they're not comparable in Go.
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
