// SPDX-License-Identifier: Apache-2.0

package bench

import (
	"testing"

	"github.com/sam-fredrickson/keymerge"
)

const (
	numUsers    = 100
	numServices = 50
	basePort    = 8000
)

// generateLargeBase creates a large base configuration with multiple sections.
func generateLargeBase() any {
	users := make([]any, numUsers)
	for i := 0; i < numUsers; i++ {
		users[i] = map[string]any{
			"id":    i,
			"name":  "user" + string(rune(i)),
			"email": "user" + string(rune(i)) + "@example.com",
			"role":  "member",
			"settings": map[string]any{
				"notifications": true,
				"theme":         "light",
				"language":      "en",
			},
		}
	}

	services := make([]any, numServices)
	for i := 0; i < numServices; i++ {
		services[i] = map[string]any{
			"name": "service" + string(rune(i)),
			"port": basePort + i,
			"config": map[string]any{
				"timeout":     30,
				"retries":     3,
				"compression": true,
			},
		}
	}

	return map[string]any{
		"version":  "1.0",
		"users":    users,
		"services": services,
		"global": map[string]any{
			"debug":   false,
			"logging": "info",
			"region":  "us-east-1",
		},
	}
}

// generateOverlays creates multiple overlays that touch different parts of the config.
func generateOverlays(count int) []any {
	overlays := make([]any, count)
	for i := 0; i < count; i++ {
		// Each overlay updates different users and services
		overlays[i] = map[string]any{
			"users": []any{
				map[string]any{
					"id":   i * 2,
					"role": "admin",
				},
				map[string]any{
					"id": i*2 + 1,
					"settings": map[string]any{
						"theme": "dark",
					},
				},
			},
			"services": []any{
				map[string]any{
					"name": "service" + string(rune(i)),
					"config": map[string]any{
						"timeout": 60,
					},
				},
			},
		}
	}
	return overlays
}

func BenchmarkMerge_Small(b *testing.B) {
	opts := keymerge.Options{PrimaryKeyNames: []string{"id", "name"}}
	base := map[string]any{
		"users": []any{
			map[string]any{"id": 1, "name": "alice"},
			map[string]any{"id": 2, "name": "bob"},
		},
	}
	overlay := map[string]any{
		"users": []any{
			map[string]any{"id": 1, "role": "admin"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, base, overlay)
	}
}

func BenchmarkMerge_Medium(b *testing.B) {
	opts := keymerge.Options{PrimaryKeyNames: []string{"id", "name"}}
	base := generateLargeBase()
	overlays := generateOverlays(5)

	docs := make([]any, len(overlays)+1)
	docs[0] = base
	copy(docs[1:], overlays)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, docs...)
	}
}

func BenchmarkMerge_Large(b *testing.B) {
	opts := keymerge.Options{PrimaryKeyNames: []string{"id", "name"}}
	base := generateLargeBase()
	overlays := generateOverlays(20)

	docs := make([]any, len(overlays)+1)
	docs[0] = base
	copy(docs[1:], overlays)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, docs...)
	}
}

func BenchmarkMerge_DeepNesting(b *testing.B) {
	opts := keymerge.Options{PrimaryKeyNames: []string{"id"}}

	// Create deeply nested structure
	base := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"level4": map[string]any{
						"items": []any{
							map[string]any{"id": 1, "value": "a"},
							map[string]any{"id": 2, "value": "b"},
						},
					},
				},
			},
		},
	}

	overlay := map[string]any{
		"level1": map[string]any{
			"level2": map[string]any{
				"level3": map[string]any{
					"level4": map[string]any{
						"items": []any{
							map[string]any{"id": 1, "value": "updated"},
							map[string]any{"id": 3, "value": "c"},
						},
					},
				},
			},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, base, overlay)
	}
}

func BenchmarkMerge_ListsWithoutPrimaryKeys(b *testing.B) {
	opts := keymerge.Options{PrimaryKeyNames: []string{"id"}}

	base := map[string]any{
		"tags": []any{"tag1", "tag2", "tag3", "tag4", "tag5"},
	}

	overlay := map[string]any{
		"tags": []any{"tag6", "tag7", "tag8"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, base, overlay)
	}
}

func BenchmarkMerge_ManySmallOverlays(b *testing.B) {
	opts := keymerge.Options{PrimaryKeyNames: []string{"id"}}
	base := generateLargeBase()
	overlays := generateOverlays(50)

	docs := make([]any, len(overlays)+1)
	docs[0] = base
	copy(docs[1:], overlays)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, docs...)
	}
}

func BenchmarkMerge_ScalarOverridesOnly(b *testing.B) {
	opts := keymerge.Options{PrimaryKeyNames: []string{"id"}}

	base := map[string]any{
		"a": 1,
		"b": 2,
		"c": 3,
		"d": 4,
		"e": 5,
		"f": map[string]any{
			"g": 6,
			"h": 7,
			"i": 8,
		},
	}

	overlay := map[string]any{
		"a": 10,
		"c": 30,
		"f": map[string]any{
			"h": 70,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, base, overlay)
	}
}

func BenchmarkMerge_ScalarListDedup_Small(b *testing.B) {
	opts := keymerge.Options{
		ScalarListMode: keymerge.ScalarListDedup,
	}

	base := map[string]any{
		"tags": []any{"a", "b", "c", "d", "e"},
	}
	overlay := map[string]any{
		"tags": []any{"c", "d", "e", "f", "g"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, base, overlay)
	}
}

func BenchmarkMerge_ScalarListDedup_Medium(b *testing.B) {
	opts := keymerge.Options{
		ScalarListMode: keymerge.ScalarListDedup,
	}

	// 50 items in base, 50 in overlay with 25 duplicates
	baseTags := make([]any, 50)
	overlayTags := make([]any, 50)
	for i := 0; i < 50; i++ {
		baseTags[i] = i
		if i < 25 {
			overlayTags[i] = i // Duplicates
		} else {
			overlayTags[i] = i + 50 // New items
		}
	}

	base := map[string]any{"tags": baseTags}
	overlay := map[string]any{"tags": overlayTags}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, base, overlay)
	}
}

func BenchmarkMerge_ScalarListDedup_Large(b *testing.B) {
	opts := keymerge.Options{
		ScalarListMode: keymerge.ScalarListDedup,
	}

	// 200 items in base, 200 in overlay with 100 duplicates
	baseTags := make([]any, 200)
	overlayTags := make([]any, 200)
	for i := 0; i < 200; i++ {
		baseTags[i] = i
		if i < 100 {
			overlayTags[i] = i // Duplicates
		} else {
			overlayTags[i] = i + 200 // New items
		}
	}

	base := map[string]any{"tags": baseTags}
	overlay := map[string]any{"tags": overlayTags}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = keymerge.Merge(opts, base, overlay)
	}
}
