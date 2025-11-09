package keymerge

import (
	"fmt"
	"reflect"
	"strings"
)

// TagKind identifies which km struct tag directive had an error.
type TagKind int

const (
	// UnknownTag indicates an unknown or unsupported km tag directive.
	UnknownTag TagKind = iota
	// PrimaryTag indicates an error with km:"primary" directive.
	PrimaryTag
	// ModeTag indicates an error with km:"mode=..." directive.
	ModeTag
	// DupeTag indicates an error with km:"dupe=..." directive.
	DupeTag
	// FieldTag indicates an error with km:"field=..." directive.
	FieldTag
)

func (k TagKind) String() string {
	switch k {
	case UnknownTag:
		return "unknown"
	case PrimaryTag:
		return "primary"
	case ModeTag:
		return "mode"
	case DupeTag:
		return "dupe"
	case FieldTag:
		return "field"
	default:
		return fmt.Sprintf("TagKind(%d)", k)
	}
}

// InvalidTagError is returned when a km struct tag contains an invalid directive or value.
type InvalidTagError struct {
	// Kind indicates which km tag directive had the error.
	Kind TagKind
	// FieldName is the struct field name where the error occurred.
	FieldName string
	// Value is the invalid value (e.g., the invalid mode string).
	Value string
	// Message provides details about what went wrong.
	Message string
}

func (e *InvalidTagError) Error() string {
	if e.Value != "" {
		return fmt.Sprintf("field %s: invalid %s tag: %s (value: %q)",
			e.FieldName, e.Kind.String(), e.Message, e.Value)
	}
	return fmt.Sprintf("field %s: invalid %s tag: %s",
		e.FieldName, e.Kind.String(), e.Message)
}

func (e *InvalidTagError) Is(target error) bool {
	return target == ErrInvalidTag
}

// Merger is a type-safe merger that uses reflection to extract merge directives
// from struct tags.
//
// It embeds a [UntypedMerger] and inherits all its methods. The type parameter T is used
// to build metadata from struct tags at creation time, enabling fine-grained control
// over merge behavior for different fields.
//
// Struct tag format:
//   - km:"primary" - marks a field as part of the composite primary key (only affects list item matching)
//   - km:"mode=concat|dedup|replace" - sets scalar list merge mode for this field
//   - km:"dupe=unique|consolidate" - sets object list mode for this field
//   - km:"field=name" - overrides field name detection (for non-standard serialization)
//
// Multiple directives can be combined: km:"field=wtfs,dupe=consolidate"
//
// Field names are automatically detected from yaml, json, and toml struct tags.
//
// Note: The km:"primary" tag only affects merging when the struct type is used as a list item type.
// For example, if Service has km:"primary" tags, they're used when merging []Service lists.
// Primary key tags on root-level fields or non-list fields have no effect.
//
// Example:
//
//	type Config struct {
//		Foos []Foo    `yaml:"foos" km:"dupe=consolidate"`
//		Bars []string `yaml:"bars" km:"mode=dedup"`
//	}
//
//	type Foo struct {
//		ID   string `yaml:"id" km:"primary"`
//		Name string `yaml:"name" km:"primary"`  // composite key with ID
//		URL  string `yaml:"url"`
//	}
//
//	merger, _ := NewMerger[Config](Options{})
//	result, _ := merger.MergeMarshal(yaml.Unmarshal, yaml.Marshal, doc1, doc2)
type Merger[T any] struct {
	*UntypedMerger
}

// NewMerger creates a new [Merger] with metadata extracted from type T's struct tags.
//
// The type parameter T should be a struct type with km struct tags specifying merge behavior.
// The Options provide default behavior for fields without specific tags.
//
// Returns an error if the options are invalid or if struct tags contain invalid directives.
func NewMerger[T any](opts Options) (*Merger[T], error) {
	merger, err := NewUntypedMerger(opts)
	if err != nil {
		return nil, err
	}

	// Build metadata tree from T's reflection
	metadata, err := buildMetadata(reflect.TypeOf((*T)(nil)).Elem())
	if err != nil {
		return nil, err
	}

	merger.metadata = metadata

	return &Merger[T]{UntypedMerger: merger}, nil
}

// buildMetadata recursively builds a metadata tree from a type's struct tags.
func buildMetadata(t reflect.Type) (*fieldMetadata, error) {
	// Non-struct types have no metadata
	if t.Kind() != reflect.Struct {
		return &fieldMetadata{}, nil
	}

	root := &fieldMetadata{
		children: make(map[string]*fieldMetadata),
	}

	// Process each field in the struct
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get the serialized field name
		fieldName, err := getFieldName(field)
		if err != nil {
			return nil, err
		}

		// Parse km tag directives
		meta := &fieldMetadata{
			fieldName: fieldName,
		}

		kmTag := field.Tag.Get("km")
		if kmTag != "" {
			if err := parseKMTag(kmTag, meta); err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}
		}

		// Validate that primary key fields are comparable types
		for _, pk := range meta.primaryKeys {
			if pk == fieldName {
				// This field is marked as primary, check if it's comparable
				if !field.Type.Comparable() {
					return nil, &InvalidTagError{
						Kind:      PrimaryTag,
						FieldName: field.Name,
						Message:   fmt.Sprintf("primary key field must be comparable type, got %s", field.Type.String()),
					}
				}
			}
		}

		// Recursively process nested types
		fieldType := field.Type
		// Unwrap pointer and slice types to get to the underlying type
		for fieldType.Kind() == reflect.Ptr || fieldType.Kind() == reflect.Slice {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct {
			children, err := buildMetadata(fieldType)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", field.Name, err)
			}
			meta.children = children.children
			// If the child type has primary keys defined, inherit them
			if len(children.primaryKeys) > 0 {
				meta.primaryKeys = children.primaryKeys
			}
		}

		root.children[fieldName] = meta
	}

	// Collect primary key fields defined at THIS struct level only
	// (not from nested structs - those are already in their respective meta.primaryKeys)
	var primaryKeys []string
	for fieldName, meta := range root.children {
		// Check if THIS field itself is marked as primary
		// (meta.primaryKeys contains its own name if it was marked with km:"primary")
		for _, pk := range meta.primaryKeys {
			if pk == fieldName {
				primaryKeys = append(primaryKeys, fieldName)
				break
			}
		}
	}
	root.primaryKeys = primaryKeys

	return root, nil
}

// getFieldName extracts the serialized field name from struct tags.
// Priority: km:field override > yaml > json > toml > struct field name.
func getFieldName(field reflect.StructField) (string, error) {
	// Check km tag for explicit field name override
	if kmTag := field.Tag.Get("km"); kmTag != "" {
		fieldName, err := extractFieldDirective(kmTag)
		if err != nil {
			return "", fmt.Errorf("field %s: %w", field.Name, err)
		}
		if fieldName != "" {
			return fieldName, nil
		}
	}

	// Check common serialization tags
	for _, tagName := range []string{"yaml", "json", "toml"} {
		if tag := field.Tag.Get(tagName); tag != "" && tag != "-" {
			// Handle "name,omitempty,inline" format - take first part
			if idx := strings.Index(tag, ","); idx != -1 {
				return tag[:idx], nil
			}
			return tag, nil
		}
	}

	// Fall back to struct field name
	return field.Name, nil
}

// extractFieldDirective extracts the field=name directive from a km tag.
// Returns the field name and any validation error.
func extractFieldDirective(kmTag string) (string, error) {
	parts := strings.Split(kmTag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "field=") {
			fieldName := strings.TrimPrefix(part, "field=")
			if fieldName == "" {
				return "", &InvalidTagError{
					Kind:    FieldTag,
					Value:   part,
					Message: "field name cannot be empty",
				}
			}
			return fieldName, nil
		}
	}
	return "", nil
}

// parseKMTag parses the km struct tag and populates the fieldMetadata.
func parseKMTag(tag string, meta *fieldMetadata) error {
	parts := strings.Split(tag, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Handle primary key marker
		if part == "primary" {
			// Mark this field as a primary key by adding its own name
			meta.primaryKeys = append(meta.primaryKeys, meta.fieldName)
			continue
		}

		// Handle mode=value directives
		if strings.HasPrefix(part, "mode=") {
			modeStr := strings.TrimPrefix(part, "mode=")
			mode, err := parseScalarListMode(modeStr, meta.fieldName)
			if err != nil {
				return err
			}
			meta.scalarListMode = &mode
			continue
		}

		// Handle dupe=value directives
		if strings.HasPrefix(part, "dupe=") {
			dupeStr := strings.TrimPrefix(part, "dupe=")
			mode, err := parseObjectListMode(dupeStr, meta.fieldName)
			if err != nil {
				return err
			}
			meta.objectListMode = &mode
			continue
		}

		// field= is handled separately in getFieldName, skip it here
		if strings.HasPrefix(part, "field=") {
			continue
		}

		// Unknown directive
		return &InvalidTagError{
			Kind:      UnknownTag,
			FieldName: meta.fieldName,
			Value:     part,
			Message:   "unknown km tag directive",
		}
	}

	return nil
}

// parseScalarListMode converts a string to ScalarListMode.
func parseScalarListMode(s string, fieldName string) (ScalarListMode, error) {
	switch s {
	case "concat":
		return ScalarListConcat, nil
	case "dedup":
		return ScalarListDedup, nil
	case "replace":
		return ScalarListReplace, nil
	default:
		return 0, &InvalidTagError{
			Kind:      ModeTag,
			FieldName: fieldName,
			Value:     s,
			Message:   "valid: concat, dedup, replace",
		}
	}
}

// parseObjectListMode converts a string to ObjectListMode.
func parseObjectListMode(s string, fieldName string) (ObjectListMode, error) {
	switch s {
	case "unique":
		return ObjectListUnique, nil
	case "consolidate":
		return ObjectListConsolidate, nil
	default:
		return 0, &InvalidTagError{
			Kind:      DupeTag,
			FieldName: fieldName,
			Value:     s,
			Message:   "valid: unique, consolidate",
		}
	}
}
