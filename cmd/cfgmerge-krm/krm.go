// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/goccy/go-yaml"

	"github.com/sam-fredrickson/keymerge"
)

// KRM annotation constants.
const (
	// AnnotationBase is the base prefix for all keymerge annotations.
	AnnotationBase = "config.keymerge.io/"

	// AnnotationID is a correlation key grouping ConfigMaps for a single merge operation.
	AnnotationID = AnnotationBase + "id"

	// AnnotationOrder defines the merge order for ConfigMaps with the same ID.
	// Lower numbers are merged first. The ConfigMap with order=0 is the base.
	AnnotationOrder = AnnotationBase + "order"

	// AnnotationFinalName specifies the desired metadata.name of the final merged ConfigMap.
	// Must be present on the base ConfigMap (order=0).
	AnnotationFinalName = AnnotationBase + "final-name"

	// AnnotationKeys specifies comma-separated primary key names for this ConfigMap.
	// Overrides global defaults. Example: "id,name,uuid".
	AnnotationKeys = AnnotationBase + "keys"

	// AnnotationScalarMode specifies scalar list merge mode: concat, dedup, or replace.
	AnnotationScalarMode = AnnotationBase + "scalar-mode"

	// AnnotationDupeMode specifies object list duplicate handling: unique or consolidate.
	AnnotationDupeMode = AnnotationBase + "dupe-mode"

	// AnnotationDeleteMarker specifies the deletion marker key.
	AnnotationDeleteMarker = AnnotationBase + "delete-marker"
)

// TypeMeta describes an individual object in a ResourceList.
type TypeMeta struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
}

// ObjectMeta is metadata that all persisted resources must have.
type ObjectMeta struct {
	Name        string            `yaml:"name,omitempty" json:"name,omitempty"`
	Namespace   string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Annotations map[string]string `yaml:"annotations,omitempty" json:"annotations,omitempty"`
}

// ConfigMap represents a Kubernetes ConfigMap resource.
type ConfigMap struct {
	TypeMeta   `yaml:",inline" json:",inline"`
	ObjectMeta `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	Data       map[string]string `yaml:"data,omitempty" json:"data,omitempty"`
}

// ResourceList is the input/output format for KRM functions.
// See: https://github.com/kubernetes-sigs/kustomize/blob/master/cmd/config/docs/api-conventions/functions-spec.md
type ResourceList struct {
	APIVersion string           `yaml:"apiVersion" json:"apiVersion"`
	Kind       string           `yaml:"kind" json:"kind"`
	Items      []map[string]any `yaml:"items" json:"items"`
}

// configMapGroup represents a set of ConfigMaps with the same ID that need to be merged.
type configMapGroup struct {
	id          string
	configMaps  []*configMapWithOrder
	baseOptions keymerge.Options // Options from the base (order=0) ConfigMap
}

// configMapWithOrder wraps a ConfigMap with its merge order and per-ConfigMap options.
type configMapWithOrder struct {
	order     int
	configMap ConfigMap
	options   keymerge.Options // Per-ConfigMap merge options
	finalName string           // Only set on base (order=0)
}

// Run executes the KRM plugin mode, reading a ResourceList from stdin and writing to stdout.
func Run(in io.Reader, out io.Writer) error {
	// Read ResourceList from stdin
	rl, err := readResourceList(in)
	if err != nil {
		return fmt.Errorf("failed to read ResourceList: %w", err)
	}

	// Group ConfigMaps by annotation ID
	groups, passthrough, err := groupConfigMaps(rl)
	if err != nil {
		return fmt.Errorf("failed to group ConfigMaps: %w", err)
	}

	// Merge each group
	mergedConfigMaps := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		merged, err := mergeConfigMapGroup(group)
		if err != nil {
			return fmt.Errorf("failed to merge ConfigMap group %q: %w", group.id, err)
		}
		mergedConfigMaps = append(mergedConfigMaps, merged)
	}

	// Construct output ResourceList
	outputRL := ResourceList{
		APIVersion: "v1",
		Kind:       "ResourceList",
		Items:      append(passthrough, mergedConfigMaps...),
	}

	// Write to stdout
	if err := writeResourceList(out, outputRL); err != nil {
		return fmt.Errorf("failed to write ResourceList: %w", err)
	}

	return nil
}

// readResourceList reads and unmarshals a ResourceList from a reader.
func readResourceList(r io.Reader) (*ResourceList, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	var rl ResourceList
	if err := yaml.Unmarshal(data, &rl); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ResourceList: %w", err)
	}

	return &rl, nil
}

// writeResourceList marshals and writes a ResourceList to a writer.
func writeResourceList(w io.Writer, rl ResourceList) error {
	data, err := yaml.Marshal(rl)
	if err != nil {
		return fmt.Errorf("failed to marshal ResourceList: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

// groupConfigMaps separates ConfigMaps with keymerge annotations from passthrough resources.
func groupConfigMaps(rl *ResourceList) (map[string]*configMapGroup, []map[string]any, error) {
	groups := make(map[string]*configMapGroup)
	var passthrough []map[string]any

	for _, item := range rl.Items {
		// Check if this is a ConfigMap with keymerge ID annotation
		cm, isConfigMap, err := parseConfigMap(item)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse resource: %w", err)
		}

		if !isConfigMap {
			passthrough = append(passthrough, item)
			continue
		}

		id, ok := cm.Annotations[AnnotationID]
		if !ok || id == "" {
			// ConfigMap without keymerge ID - passthrough
			passthrough = append(passthrough, item)
			continue
		}

		// Parse annotations
		cmWithOrder, err := parseConfigMapAnnotations(cm)
		if err != nil {
			return nil, nil, fmt.Errorf("ConfigMap %q: %w", cm.Name, err)
		}

		// Add to group
		if groups[id] == nil {
			groups[id] = &configMapGroup{
				id:         id,
				configMaps: make([]*configMapWithOrder, 0),
			}
		}
		groups[id].configMaps = append(groups[id].configMaps, cmWithOrder)
	}

	// Sort each group by order and validate
	for id, group := range groups {
		if err := prepareGroup(group); err != nil {
			return nil, nil, fmt.Errorf("ConfigMap group %q: %w", id, err)
		}
	}

	return groups, passthrough, nil
}

// parseConfigMap attempts to parse a resource item as a ConfigMap.
func parseConfigMap(item map[string]any) (ConfigMap, bool, error) {
	// Check if this is a ConfigMap
	apiVersion, _ := item["apiVersion"].(string)
	kind, _ := item["kind"].(string)

	if kind != "ConfigMap" {
		return ConfigMap{}, false, nil
	}

	// Marshal and unmarshal to convert map to ConfigMap struct
	data, err := yaml.Marshal(item)
	if err != nil {
		return ConfigMap{}, false, fmt.Errorf("failed to marshal item: %w", err)
	}

	var cm ConfigMap
	if err := yaml.Unmarshal(data, &cm); err != nil {
		return ConfigMap{}, false, fmt.Errorf("failed to unmarshal ConfigMap: %w", err)
	}

	// Ensure apiVersion and kind are set
	if cm.APIVersion == "" {
		cm.APIVersion = apiVersion
	}
	if cm.Kind == "" {
		cm.Kind = kind
	}

	return cm, true, nil
}

// parseConfigMapAnnotations extracts keymerge annotations from a ConfigMap.
func parseConfigMapAnnotations(cm ConfigMap) (*configMapWithOrder, error) {
	annotations := cm.Annotations
	if annotations == nil {
		return nil, fmt.Errorf("missing required annotation %q", AnnotationOrder)
	}

	// Parse order (required)
	orderStr, ok := annotations[AnnotationOrder]
	if !ok || orderStr == "" {
		return nil, fmt.Errorf("missing required annotation %q", AnnotationOrder)
	}

	order, err := strconv.Atoi(orderStr)
	if err != nil {
		return nil, fmt.Errorf("invalid %q annotation: %w", AnnotationOrder, err)
	}

	// Parse final-name (required for base, ignored otherwise)
	finalName := annotations[AnnotationFinalName]

	// Parse merge options (optional, with defaults)
	opts, err := parseMergeOptions(annotations)
	if err != nil {
		return nil, fmt.Errorf("failed to parse merge options: %w", err)
	}

	return &configMapWithOrder{
		order:     order,
		configMap: cm,
		options:   opts,
		finalName: finalName,
	}, nil
}

// parseMergeOptions extracts keymerge.Options from annotations.
func parseMergeOptions(annotations map[string]string) (keymerge.Options, error) {
	opts := keymerge.Options{
		PrimaryKeyNames: []string{"name", "id"}, // Default
		ScalarMode:      keymerge.ScalarConcat,  // Default
		DupeMode:        keymerge.DupeUnique,    // Default
		DeleteMarkerKey: "_delete",              // Default
	}

	// Parse primary keys
	if keys, ok := annotations[AnnotationKeys]; ok && keys != "" {
		opts.PrimaryKeyNames = strings.Split(keys, ",")
		// Trim whitespace from each key
		for i := range opts.PrimaryKeyNames {
			opts.PrimaryKeyNames[i] = strings.TrimSpace(opts.PrimaryKeyNames[i])
		}
	}

	// Parse scalar mode
	if modeStr, ok := annotations[AnnotationScalarMode]; ok && modeStr != "" {
		mode, err := parseScalarModeString(modeStr)
		if err != nil {
			return opts, fmt.Errorf("invalid %q annotation: %w", AnnotationScalarMode, err)
		}
		opts.ScalarMode = mode
	}

	// Parse dupe mode
	if modeStr, ok := annotations[AnnotationDupeMode]; ok && modeStr != "" {
		mode, err := parseDupeModeString(modeStr)
		if err != nil {
			return opts, fmt.Errorf("invalid %q annotation: %w", AnnotationDupeMode, err)
		}
		opts.DupeMode = mode
	}

	// Parse delete marker
	if marker, ok := annotations[AnnotationDeleteMarker]; ok && marker != "" {
		opts.DeleteMarkerKey = marker
	}

	return opts, nil
}

// parseScalarModeString converts a string to keymerge.ScalarMode.
func parseScalarModeString(s string) (keymerge.ScalarMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "concat":
		return keymerge.ScalarConcat, nil
	case "dedup":
		return keymerge.ScalarDedup, nil
	case "replace":
		return keymerge.ScalarReplace, nil
	default:
		return keymerge.ScalarConcat, fmt.Errorf("unknown scalar mode %q (must be concat, dedup, or replace)", s)
	}
}

// parseDupeModeString converts a string to keymerge.DupeMode.
func parseDupeModeString(s string) (keymerge.DupeMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "unique":
		return keymerge.DupeUnique, nil
	case "consolidate":
		return keymerge.DupeConsolidate, nil
	default:
		return keymerge.DupeUnique, fmt.Errorf("unknown dupe mode %q (must be unique or consolidate)", s)
	}
}

// prepareGroup sorts a group by order and validates it.
func prepareGroup(group *configMapGroup) error {
	// Sort by order
	slices.SortFunc(group.configMaps, func(a, b *configMapWithOrder) int {
		return a.order - b.order
	})

	if len(group.configMaps) == 0 {
		return fmt.Errorf("empty ConfigMap group")
	}

	// Validate base ConfigMap (order=0)
	base := group.configMaps[0]
	if base.order != 0 {
		return fmt.Errorf("no base ConfigMap with order=0 (lowest order is %d)", base.order)
	}

	if base.finalName == "" {
		return fmt.Errorf("base ConfigMap %q missing required annotation %q", base.configMap.Name, AnnotationFinalName)
	}

	// Store base options at group level
	group.baseOptions = base.options

	return nil
}

// mergeConfigMapGroup merges all ConfigMaps in a group into a single ConfigMap.
func mergeConfigMapGroup(group *configMapGroup) (map[string]any, error) {
	base := group.configMaps[0]

	// Collect all data keys from all ConfigMaps
	allKeys := make(map[string]struct{})
	for _, cm := range group.configMaps {
		for key := range cm.configMap.Data {
			allKeys[key] = struct{}{}
		}
	}

	// Convert to sorted slice for deterministic ordering
	keysToMerge := make([]string, 0, len(allKeys))
	for key := range allKeys {
		keysToMerge = append(keysToMerge, key)
	}
	slices.Sort(keysToMerge)

	// Merge all data keys
	mergedData := make(map[string]string)
	for _, dataKey := range keysToMerge {
		merged, err := mergeDataKey(group, dataKey)
		if err != nil {
			return nil, fmt.Errorf("failed to merge data key %q: %w", dataKey, err)
		}
		if merged != "" {
			mergedData[dataKey] = merged
		}
	}

	// Create final ConfigMap
	result := ConfigMap{
		TypeMeta: TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: ObjectMeta{
			Name:      base.finalName,
			Namespace: base.configMap.Namespace,
			// Don't include keymerge annotations in final output
			Annotations: filterKeymergeAnnotations(base.configMap.Annotations),
			Labels:      base.configMap.Labels,
		},
		Data: mergedData,
	}

	// Convert to map[string]any for ResourceList
	data, err := yaml.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged ConfigMap: %w", err)
	}

	var resultMap map[string]any
	if err := yaml.Unmarshal(data, &resultMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal merged ConfigMap: %w", err)
	}

	return resultMap, nil
}

// mergeDataKey merges a single data key across all ConfigMaps in a group.
func mergeDataKey(group *configMapGroup, dataKey string) (string, error) {
	// Collect all values for this data key, along with their options.
	// We need parallel slices because not all ConfigMaps have every data key.
	var contents [][]byte
	var options []keymerge.Options
	var cmNames []string
	for _, cm := range group.configMaps {
		if value, ok := cm.configMap.Data[dataKey]; ok && value != "" {
			contents = append(contents, []byte(value))
			options = append(options, cm.options)
			cmNames = append(cmNames, cm.configMap.Name)
		}
	}

	if len(contents) == 0 {
		return "", nil // No data for this key
	}

	if len(contents) == 1 {
		return string(contents[0]), nil // No merge needed
	}

	// Detect format from data key name
	unmarshal, formatName, err := detectFormatFromKey(dataKey)
	if err != nil {
		return "", fmt.Errorf("data key %q: %w", dataKey, err)
	}

	// Merge sequentially: base + overlay1 + overlay2 + ...
	// Each step can use different merge options from the overlay ConfigMap
	result := contents[0]
	for i := 1; i < len(contents); i++ {
		opts := options[i] // Use per-ConfigMap options (aligned with contents)

		// Merge
		merged, err := keymerge.Merge(opts, unmarshal, yaml.Marshal, result, contents[i])
		if err != nil {
			return "", fmt.Errorf("ConfigMap %q (format: %s): %w",
				cmNames[i], formatName, err)
		}

		result = merged
	}

	return string(result), nil
}

// detectFormatFromKey detects the format based on the data key name (e.g., "config.yaml" → YAML).
func detectFormatFromKey(dataKey string) (func([]byte, any) error, string, error) {
	ext := strings.ToLower(filepath.Ext(dataKey))

	switch ext {
	case ".yaml", ".yml":
		return yaml.Unmarshal, "yaml", nil
	case ".json":
		return json.Unmarshal, "json", nil
	case ".toml":
		return toml.Unmarshal, "toml", nil
	default:
		// Default to YAML for keys without extension (common in Kubernetes)
		return yaml.Unmarshal, "yaml (default)", nil
	}
}

// filterKeymergeAnnotations removes keymerge.io annotations from a map.
func filterKeymergeAnnotations(annotations map[string]string) map[string]string {
	if annotations == nil {
		return nil
	}

	filtered := make(map[string]string)
	for key, value := range annotations {
		if !strings.HasPrefix(key, AnnotationBase) {
			filtered[key] = value
		}
	}

	if len(filtered) == 0 {
		return nil
	}

	return filtered
}
