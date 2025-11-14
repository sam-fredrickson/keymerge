// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/goccy/go-yaml"

	"github.com/sam-fredrickson/keymerge"
)

var version = "dev"

func main() {
	var failed bool
	defer func() {
		if failed {
			os.Exit(1)
		}
	}()

	program := os.Args[0]
	var keys primaryKeys
	var scalar scalarMode
	var dupe dupeMode
	var deleteMarker string
	var outputPath string
	var outputFormat format
	var showVersion bool

	flag.Usage = func() {
		out := flag.CommandLine.Output()
		fmt.Fprintf(out, "usage: %s [flags] FILE...\n\n", program)
		fmt.Fprintf(out, "Merges configuration files (YAML, JSON, TOML) with intelligent list handling.\n")
		fmt.Fprintf(out, "Items in lists are matched by primary key fields and deep-merged.\n\n")
		fmt.Fprintf(out, "Example:\n")
		fmt.Fprintf(out, "  # merge env-specific overlay into common base\n")
		fmt.Fprintf(out, "  %s -out config.yaml base.yaml env.yaml\n\n", program)
		fmt.Fprintf(out, "  # merge general prod overlay and env-specific overlay into common base\n")
		fmt.Fprintf(out, "  %s -out config.yaml base.yaml prod.yaml env.yaml\n\n", program)
		fmt.Fprintf(out, "Flags:\n")
		flag.PrintDefaults()
	}

	flag.Var(&keys, "keys", `comma-separated list of primary keys (default "name,id")`)
	flag.Var(&scalar, "scalar", `scalar list mode [concat, dedup, replace] (default "concat")`)
	flag.Var(&dupe, "dupe", `list dupe mode [unique, consolidate] (default "unique")`)
	flag.StringVar(&deleteMarker, "delete-marker", "_delete", "deletion marker key")
	flag.StringVar(&outputPath, "out", "", "output file path (defaults to stdout)")
	flag.Var(&outputFormat, "format", `output format [json, yaml, toml] (defaults to first file's format)`)
	flag.BoolVar(&showVersion, "version", false, "show version and exit")
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		return
	}

	files := flag.Args()
	var output io.Writer
	if outputPath != "" {
		f, err := os.Create(outputPath)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			failed = true
			return
		}
		defer f.Close()
		output = f
	} else {
		output = os.Stdout
	}

	err := Run(
		keys, scalar, dupe, deleteMarker,
		files, outputFormat,
		output,
	)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		_, _ = fmt.Fprintf(os.Stderr, "usage: %s [flags] FILE...\n", program)
		failed = true
		return
	}
}

func Run(
	keys primaryKeys,
	scalar scalarMode,
	dupe dupeMode,
	deleteMarker string,
	files []string,
	outputFormat format,
	output io.Writer,
) error {
	if len(files) == 0 {
		return fmt.Errorf("no files to merge")
	}
	if len(keys) == 0 {
		keys = []string{"name", "id"}
	}
	opts := keymerge.Options{
		PrimaryKeyNames: keys.Keys(),
		DeleteMarkerKey: deleteMarker,
		ScalarMode:      scalar.Mode(),
		DupeMode:        dupe.Mode(),
	}

	var docs []any
	for _, file := range files {
		var doc any
		fileFormat, err := unmarshalFile(file, &doc)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", file, err)
		}
		docs = append(docs, doc)
		if outputFormat == "" {
			outputFormat = fileFormat
		}
	}

	merged, err := keymerge.MergeUnstructured(opts, docs...)
	if err != nil {
		return fmt.Errorf("merge failed while processing files %v: %w", files, err)
	}

	marshaled, err := outputFormat.Marshal(merged)
	if err != nil {
		return fmt.Errorf("failed to marshal result as %s: %w", outputFormat, err)
	}

	_, err = output.Write(marshaled)
	if err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	return nil
}

func unmarshalFile(file string, out any) (format, error) {
	var f format

	contents, err := os.ReadFile(file)
	if err != nil {
		return f, err
	}

	extension := filepath.Ext(file)
	extension = strings.ToLower(extension)
	var unmarshal func([]byte, any) error
	switch extension {
	case ".yaml", ".yml":
		f = validFormats["yaml"]
		unmarshal = yaml.Unmarshal
	case ".json":
		f = validFormats["json"]
		unmarshal = json.Unmarshal
	case ".toml":
		f = validFormats["toml"]
		unmarshal = toml.Unmarshal
	}
	if unmarshal == nil {
		return f, fmt.Errorf("unsupported file format: %s", extension)
	}

	err = unmarshal(contents, out)
	if err != nil {
		return f, err
	}

	return f, nil
}

type primaryKeys []string

func (c *primaryKeys) String() string {
	return strings.Join(*c, ",")
}

func (c *primaryKeys) Set(value string) error {
	*c = append(*c, strings.Split(value, ",")...)
	return nil
}

func (c *primaryKeys) Keys() []string {
	return *c
}

type scalarMode keymerge.ScalarMode

func (s *scalarMode) String() string {
	mode := keymerge.ScalarMode(*s)
	return mode.String()
}

func (s *scalarMode) Set(value string) error {
	var mode keymerge.ScalarMode
	switch value {
	case "":
		break
	case "concat":
		break
	case "dedup":
		mode = keymerge.ScalarDedup
	case "replace":
		mode = keymerge.ScalarReplace
	default:
		return fmt.Errorf("scalar mode %q is invalid", value)
	}
	*s = scalarMode(mode)
	return nil
}

func (s *scalarMode) Mode() keymerge.ScalarMode {
	return keymerge.ScalarMode(*s)
}

type dupeMode keymerge.DupeMode

func (d *dupeMode) String() string {
	mode := keymerge.DupeMode(*d)
	return mode.String()
}

func (d *dupeMode) Set(value string) error {
	var mode keymerge.DupeMode
	switch value {
	case "":
		break
	case "unique":
		break
	case "consolidate":
		mode = keymerge.DupeConsolidate
	default:
		return fmt.Errorf("dupe mode %q is invalid", value)
	}
	*d = dupeMode(mode)
	return nil
}

func (d *dupeMode) Mode() keymerge.DupeMode {
	return keymerge.DupeMode(*d)
}

type format string

var validFormats = map[string]format{
	"":     format(""),
	"json": format("json"),
	"yaml": format("yaml"),
	"toml": format("toml"),
}

func (f *format) String() string {
	return string(*f)
}

func (f *format) Set(value string) error {
	value = strings.ToLower(value)
	format, ok := validFormats[value]
	if !ok {
		return fmt.Errorf("invalid format %q", value)
	}
	*f = format
	return nil
}

func (f *format) Marshal(doc any) ([]byte, error) {
	switch *f {
	case "json":
		return json.MarshalIndent(doc, "", "  ")
	case "yaml":
		return yaml.Marshal(doc)
	case "toml":
		return toml.Marshal(doc)
	default:
		return nil, fmt.Errorf("invalid format %q", *f)
	}
}
