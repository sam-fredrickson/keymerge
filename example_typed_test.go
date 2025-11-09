// SPDX-License-Identifier: Apache-2.0

package keymerge_test

import (
	"fmt"
	"log"

	"github.com/goccy/go-yaml"

	"github.com/sam-fredrickson/keymerge"
)

// Example using Merger with composite primary keys and field-specific merge modes.
func ExampleMerger() {
	// Define your config structure with km tags
	type Endpoint struct {
		Region string `yaml:"region" km:"primary"`
		Name   string `yaml:"name" km:"primary"`
		URL    string `yaml:"url"`
	}

	type Config struct {
		Endpoints []Endpoint `yaml:"endpoints" km:"dupe=consolidate"`
		Tags      []string   `yaml:"tags" km:"mode=dedup"`
	}

	// Create a typed merger
	merger, err := keymerge.NewMerger[Config](keymerge.Options{})
	if err != nil {
		log.Fatal(err)
	}

	// Base configuration
	base := []byte(`
endpoints:
  - region: us-east
    name: api
    url: v1.example.com
  - region: us-west
    name: api
    url: v1-west.example.com
tags: [prod, stable]
`)

	// Overlay configuration
	overlay := []byte(`
endpoints:
  - region: us-east
    name: api
    url: v2.example.com
tags: [stable, latest]
`)

	// Merge
	result, err := merger.MergeMarshal(yaml.Unmarshal, yaml.Marshal, base, overlay)
	if err != nil {
		log.Fatal(err)
	}

	// Parse result
	var config Config
	if err := yaml.Unmarshal(result, &config); err != nil {
		log.Fatal(err)
	}

	// Display results
	for _, ep := range config.Endpoints {
		fmt.Printf("%s/%s: %s\n", ep.Region, ep.Name, ep.URL)
	}
	fmt.Printf("Tags: %v\n", config.Tags)

	// Output:
	// us-east/api: v2.example.com
	// us-west/api: v1-west.example.com
	// Tags: [prod stable latest]
}
