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
	"strconv"
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
	// ErrInvalidTag indicates a struct tag contained an invalid directive or value.
	ErrInvalidTag = errors.New("invalid tag")
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

// fieldMetadata contains merge directives for a specific field extracted from struct tags.
type fieldMetadata struct {
	// fieldName is the serialized field name (from yaml/json/toml tag or struct field name)
	fieldName string
	// primaryKeys lists field names that serve as composite primary keys for this object type
	primaryKeys []string
	// scalarListMode overrides the default scalar list merge mode
	scalarListMode *ScalarListMode
	// objectListMode overrides the default object list mode
	objectListMode *ObjectListMode
	// children contains metadata for nested struct fields (map key is the serialized field name)
	children map[string]*fieldMetadata
}

// pathSegment represents one level in the document path with its associated metadata.
type pathSegment struct {
	name string         // field name or array index
	meta *fieldMetadata // metadata at this path level (nil if no metadata)
}

// UntypedMerger performs document merging with the configured options.
// It tracks the current document path for detailed error reporting.
//
// An UntypedMerger can be safely reused for multiple merge operations.
//
// An UntypedMerger is not safe to use concurrently.
type UntypedMerger struct {
	opts      Options        // merge configuration
	path      []pathSegment  // current path in document tree for error reporting
	index     int            // current document index being processed
	metadata  *fieldMetadata // root metadata for Merger (nil for untyped UntypedMerger)
	unmarshal func([]byte, any) error
	marshal   func(any) ([]byte, error)
}

// NewUntypedMerger creates a new [UntypedMerger] with the given options.
// Returns an error if the options are invalid.
func NewUntypedMerger(opts Options,
	unmarshal func([]byte, any) error,
	marshal func(any) ([]byte, error),
) (*UntypedMerger, error) {
	for _, name := range opts.PrimaryKeyNames {
		if name == "" {
			return nil, fmt.Errorf("%w: empty string in PrimaryKeyNames", ErrInvalidOptions)
		}
	}
	return &UntypedMerger{opts: opts, marshal: marshal, unmarshal: unmarshal}, nil
}

// Options returns the merge options configured for this [UntypedMerger].
func (m *UntypedMerger) Options() Options {
	return m.opts
}

// MergeUnstructured merges multiple documents. See [UntypedMerger.MergeUnstructured] for details.
func MergeUnstructured(opts Options, docs ...any,
) (any, error) {
	m, err := NewUntypedMerger(opts, nil, nil)
	if err != nil {
		return nil, err
	}
	return m.MergeUnstructured(docs...)
}

// Merge merges byte documents using provided unmarshal and marshal functions.
// See [UntypedMerger.Merge] for details.
func Merge(
	opts Options,
	unmarshal func([]byte, any) error,
	marshal func(any) ([]byte, error),
	docs ...[]byte,
) ([]byte, error) {
	m, err := NewUntypedMerger(opts, unmarshal, marshal)
	if err != nil {
		return nil, err
	}
	return m.Merge(docs...)
}

// MergeUnstructured merges multiple documents left-to-right, with later documents taking precedence.
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
//	result, _ := MergeUnstructured(opts, base, overlay)
//	// Result: alice's role updated to "admin"
func (m *UntypedMerger) MergeUnstructured(docs ...any) (any, error) {
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

// Merge merges byte documents using provided unmarshal and marshal functions.
//
// Documents are unmarshaled, merged left-to-right with [UntypedMerger.MergeUnstructured], then marshaled back to bytes.
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
//	result, _ := Merge(opts, yaml.Unmarshal, yaml.Marshal, base, overlay)
func (m *UntypedMerger) Merge(
	docs ...[]byte,
) ([]byte, error) {
	if len(docs) == 0 {
		return []byte{}, nil
	}
	if m.unmarshal == nil || m.marshal == nil {
		return nil, fmt.Errorf("cannot merge unstructured documents without a unmarshal function")
	}

	// Parse all documents
	parsedDocs := make([]any, len(docs))
	for i, doc := range docs {
		var current any
		if err := m.unmarshal(doc, &current); err != nil {
			return nil, &MarshalError{
				Err:      err,
				DocIndex: i,
			}
		}
		parsedDocs[i] = current
	}

	// MergeUnstructured
	result, err := m.MergeUnstructured(parsedDocs...)
	if err != nil {
		return nil, err
	}

	// Marshal back
	return m.marshal(result)
}

func (m *UntypedMerger) reset(i int) {
	m.path = nil
	m.index = i
}

func (m *UntypedMerger) push(name string) {
	// Fast path for untyped merger: if there's no root metadata, there can't be any child metadata
	if m.metadata == nil {
		m.path = append(m.path, pathSegment{name: name, meta: nil})
		return
	}

	// Get parent metadata (last segment in path, or root if empty)
	var parentMeta *fieldMetadata
	if len(m.path) == 0 {
		parentMeta = m.metadata
	} else {
		parentMeta = m.path[len(m.path)-1].meta
	}

	// Determine metadata for this segment
	var segmentMeta *fieldMetadata
	if isNumeric(name) {
		// For array indices, keep the parent's metadata (the list metadata)
		// This allows us to access the item type's metadata via children
		segmentMeta = parentMeta
	} else if parentMeta != nil && parentMeta.children != nil {
		// For field names, navigate to child metadata
		segmentMeta = parentMeta.children[name]
	}

	m.path = append(m.path, pathSegment{name: name, meta: segmentMeta})
}

func (m *UntypedMerger) pop() {
	if len(m.path) == 0 {
		panic("unbalanced keymerge.UntypedMerger pop")
	}
	m.path = m.path[:len(m.path)-1]
}

// pathNames extracts just the names from the path segments for error messages.
func (m *UntypedMerger) pathNames() []string {
	names := make([]string, len(m.path))
	for i, seg := range m.path {
		names[i] = seg.name
	}
	return names
}

func (m *UntypedMerger) mergeValues(base, overlay any) (any, error) {
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

func (m *UntypedMerger) mergeMaps(base, overlay map[string]any) (map[string]any, error) {
	// Pre-allocate for base size since overlay keys may overlap
	result := make(map[string]any, len(base))

	// Copy base
	for k, v := range base {
		result[k] = v
	}

	// MergeUnstructured overlay
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

func (m *UntypedMerger) mergeSlices(base, overlay []any) ([]any, error) {
	// Check if items have primary keys
	if len(overlay) == 0 {
		return base, nil
	}

	// Try to find primary key by checking overlay items until we find one.
	// This handles cases where the first item might not have a primary key
	// but subsequent items do.
	var hasKeys bool
	for _, item := range overlay {
		if m.getPrimaryKey(item) != nil {
			hasKeys = true
			break
		}
	}

	if !hasKeys {
		// No primary key found in any overlay item, merge according to ScalarListMode
		scalarMode := m.opts.ScalarListMode
		// Check metadata for override
		if meta := m.getCurrentMetadata(); meta != nil && meta.scalarListMode != nil {
			scalarMode = *meta.scalarListMode
		}

		switch scalarMode {
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

	// Get the object list mode for this context
	objectMode := m.opts.ObjectListMode
	if meta := m.getCurrentMetadata(); meta != nil && meta.objectListMode != nil {
		objectMode = *meta.objectListMode
	}

	// Build index of items by composite primary key
	result := make([]any, 0, len(base))
	// resultIndex maps primary keys to positions in result.
	// Positions remain stable during merge because we mark deletions as nil
	// rather than removing items. Filtering happens only at the end.
	resultIndex := make(map[any]int, len(base))
	for i, item := range base {
		m.push(strconv.Itoa(i))

		key := m.getPrimaryKey(item)
		if key == nil {
			result = append(result, item)
			m.pop()
			continue
		}

		// Check if key is comparable (can be used as map key)
		if !isKeyComparable(key) {
			err := &NonComparablePrimaryKeyError{
				Key:      keyString(key),
				Position: i,
				Path:     m.pathNames(),
				DocIndex: m.index,
			}
			m.pop()
			return nil, err
		}

		mapKey := toMapKey(key)
		existingIdx, exists := resultIndex[mapKey]
		if !exists {
			resultIndex[mapKey] = len(result)
			result = append(result, item)
			m.pop()
			continue
		}

		// Duplicate found!
		if objectMode == ObjectListUnique {
			err := &DuplicatePrimaryKeyError{
				Key:       keyString(key),
				Positions: []int{existingIdx, i},
				Path:      m.pathNames(),
				DocIndex:  m.index,
			}
			m.pop()
			return nil, err
		}

		// ObjectListConsolidate: merge into first occurrence
		m.pop()                           // Pop current index before merging
		m.push(strconv.Itoa(existingIdx)) // Push existing index for merge
		merged, err := m.mergeValues(result[existingIdx], item)
		m.pop()
		if err != nil {
			return nil, err
		}
		result[existingIdx] = merged
	}

	// Check for duplicates in overlay (if ObjectListUnique mode)
	if objectMode == ObjectListUnique {
		overlayKeys := make(map[any]int, len(overlay))
		for i, overlayItem := range overlay {
			m.push(strconv.Itoa(i))

			if m.isMarkedForDeletion(overlayItem) {
				m.pop()
				continue // Skip deletion markers
			}

			key := m.getPrimaryKey(overlayItem)
			if key == nil {
				m.pop()
				continue
			}

			// Check if key is comparable
			if !isKeyComparable(key) {
				err := &NonComparablePrimaryKeyError{
					Key:      keyString(key),
					Position: i,
					Path:     m.pathNames(),
					DocIndex: m.index,
				}
				m.pop()
				return nil, err
			}

			mapKey := toMapKey(key)
			if firstIdx, exists := overlayKeys[mapKey]; exists {
				err := &DuplicatePrimaryKeyError{
					Key:       keyString(key),
					Positions: []int{firstIdx, i},
					Path:      m.pathNames(),
					DocIndex:  m.index,
				}
				m.pop()
				return nil, err
			}
			overlayKeys[mapKey] = i
			m.pop()
		}
	}

	// MergeUnstructured overlay items
	for i, overlayItem := range overlay {
		m.push(strconv.Itoa(i))

		// Check if this item is marked for deletion
		if m.isMarkedForDeletion(overlayItem) {
			key := m.getPrimaryKey(overlayItem)
			if key != nil {
				mapKey := toMapKey(key)
				if idx, exists := resultIndex[mapKey]; exists {
					// Mark for deletion by setting to nil, we'll filter later
					result[idx] = nil
					delete(resultIndex, mapKey)
				}
			}
			m.pop()
			continue
		}

		key := m.getPrimaryKey(overlayItem)
		if key == nil {
			// No key, append
			result = append(result, overlayItem)
			m.pop()
			continue
		}

		// Check if key is comparable (for Consolidate mode, Unique already checked)
		if objectMode != ObjectListUnique && !isKeyComparable(key) {
			err := &NonComparablePrimaryKeyError{
				Key:      keyString(key),
				Position: i,
				Path:     m.pathNames(),
				DocIndex: m.index,
			}
			m.pop()
			return nil, err
		}

		mapKey := toMapKey(key)
		if idx, exists := resultIndex[mapKey]; exists {
			// MergeUnstructured with existing item
			m.pop()                   // Pop current index before merging
			m.push(strconv.Itoa(idx)) // Push existing index for merge
			merged, err := m.mergeValues(result[idx], overlayItem)
			m.pop()
			if err != nil {
				return nil, err
			}
			result[idx] = merged
		} else {
			// Append new item
			result = append(result, overlayItem)
			resultIndex[mapKey] = len(result) - 1
			m.pop()
		}
	}

	// Filter out nil items (deleted items or consolidated duplicates)
	if m.opts.DeleteMarkerKey != "" || objectMode == ObjectListConsolidate {
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
func (m *UntypedMerger) stripDeleteMarker(value any) any {
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

// getCurrentMetadata returns the metadata for the current path in the document tree.
// Returns nil if no metadata exists (untyped merger or path not in metadata tree).
// This is O(1) since metadata is cached in the path during push().
func (m *UntypedMerger) getCurrentMetadata() *fieldMetadata {
	if len(m.path) == 0 {
		return nil
	}
	return m.path[len(m.path)-1].meta
}

// isNumeric checks if a string represents a number (array index).
func isNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// compositeKey represents a composite primary key value for map indexing.
//
// When multiple fields are marked with km:"primary", ALL fields must be present
// and match for two items to be considered the same. This enables matching on
// combinations like {region, name} or {namespace, kind, name}.
//
// Values contains multiple key field values in field declaration order.
// This type is only used for multi-field composite keys. Single-field keys
// use the value directly without allocation.
//
// Example:
//
//	type Endpoint struct {
//	    Region string `yaml:"region" km:"primary"`
//	    Name   string `yaml:"name" km:"primary"`
//	    URL    string `yaml:"url"`
//	}
//
// Items match only when BOTH region AND name are equal.
type compositeKey struct {
	values []any
}

// getPrimaryKey extracts the primary key value from an item for use as a map key.
// Returns nil if item is not a map or doesn't have any primary key fields.
//
// For single-key cases (most common), returns the key value directly (no allocation).
// For composite keys (multiple km:"primary" tags), returns a *compositeKey that implements
// comparable operations and string formatting.
//
// For metadata-defined composite keys, ALL key fields must be present.
// For global PrimaryKeyNames (backward compatibility), returns the FIRST key that exists.
func (m *UntypedMerger) getPrimaryKey(item any) any {
	mp, ok := item.(map[string]any)
	if !ok {
		return nil
	}

	// Get metadata for the current path (which should be a list field)
	meta := m.getCurrentMetadata()

	// If metadata defines primary keys, this is a composite key - require ALL fields
	// Note: meta.primaryKeys contains the keys from the item type (inherited during buildMetadata)
	if meta != nil && len(meta.primaryKeys) > 0 {
		// Optimize single-key case to avoid allocation
		if len(meta.primaryKeys) == 1 {
			val, exists := mp[meta.primaryKeys[0]]
			if !exists || val == nil {
				return nil
			}
			return val
		}

		// Multi-key case - still need compositeKey wrapper
		values := make([]any, 0, len(meta.primaryKeys))
		for _, keyName := range meta.primaryKeys {
			val, exists := mp[keyName]
			if !exists || val == nil {
				// Missing a required key field in composite key
				return nil
			}
			values = append(values, val)
		}
		return &compositeKey{values: values}
	}

	// Fall back to global options - use FIRST matching key (backward compatibility)
	for _, keyName := range m.opts.PrimaryKeyNames {
		val, exists := mp[keyName]
		if exists && val != nil {
			return val
		}
	}

	return nil
}

// String returns a string representation of the composite key for error messages.
func (ck *compositeKey) String() string {
	return fmt.Sprintf("%v", ck.values)
}

// isComparable checks if all values in the composite key are comparable.
func (ck *compositeKey) isComparable() bool {
	for _, v := range ck.values {
		if !isComparable(v) {
			return false
		}
	}
	return true
}

// keyString formats a primary key value for error messages.
// Handles both direct values and composite keys.
func keyString(key any) string {
	if ck, ok := key.(*compositeKey); ok {
		return ck.String()
	}
	return fmt.Sprintf("%v", key)
}

// toMapKey converts a primary key value to a map key.
// For single values, returns the value directly.
// For composite keys, returns a string representation.
func toMapKey(key any) any {
	if ck, ok := key.(*compositeKey); ok {
		return fmt.Sprint(ck.values)
	}
	return key
}

// isKeyComparable checks if a primary key value is comparable.
// For single values, checks if the value type is comparable.
// For composite keys, checks if all component values are comparable.
func isKeyComparable(key any) bool {
	if ck, ok := key.(*compositeKey); ok {
		return ck.isComparable()
	}
	return isComparable(key)
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
func (m *UntypedMerger) isMarkedForDeletion(value any) bool {
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
