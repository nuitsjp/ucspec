// Package adapters is the only production package coupled to external libraries.
package adapters

import (
	"bytes"
	"encoding/json"
	"fmt"
	elk "github.com/d2lang/elk-go"
	"github.com/nuitsjp/usecase-spec/internal/compiler"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"
	"io"
	"io/fs"
	"strings"
)

func LayoutJSON(b []byte) (out []byte, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("elk-go layout panic: %v", p)
		}
	}()
	return elk.LayoutJSON(b)
}
func Codec(assets fs.FS) (compiler.Codec, error) {
	raw, err := fs.ReadFile(assets, "schemas/spec.schema.json")
	if err != nil {
		return compiler.Codec{}, err
	}
	var doc any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return compiler.Codec{}, err
	}
	c := jsonschema.NewCompiler()
	const location = "https://ucspec.invalid/spec.schema.json"
	if err = c.AddResource(location, doc); err != nil {
		return compiler.Codec{}, err
	}
	project, err := c.Compile(location + "#/$defs/project")
	if err != nil {
		return compiler.Codec{}, err
	}
	fragment, err := c.Compile(location + "#/$defs/document")
	if err != nil {
		return compiler.Codec{}, err
	}
	return compiler.Codec{Decode: decodeYAML, Validate: func(kind string, doc any) error {
		if kind == "project" {
			return project.Validate(doc)
		}
		return fragment.Validate(doc)
	}}, nil
}
func decodeYAML(b []byte, ext string) ([]byte, error) {
	if strings.EqualFold(ext, ".json") {
		return b, nil
	}
	if !strings.EqualFold(ext, ".yaml") && !strings.EqualFold(ext, ".yml") {
		return nil, fmt.Errorf("supported formats: .json, .yaml, .yml")
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	var node yaml.Node
	if err := d.Decode(&node); err != nil {
		return nil, err
	}
	if err := checkYAML(&node); err != nil {
		return nil, err
	}
	var next yaml.Node
	if err := d.Decode(&next); err != io.EOF {
		return nil, fmt.Errorf("exactly one YAML document is required")
	}
	var v any
	if err := node.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
func checkYAML(n *yaml.Node) error {
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		return fmt.Errorf("YAML anchors/aliases are not supported (line %d)", n.Line)
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag != "!!str" || k.Value == "<<" {
				return fmt.Errorf("YAML mapping keys must be strings; merge keys unsupported (line %d)", k.Line)
			}
			if seen[k.Value] {
				return fmt.Errorf("duplicate YAML key %s (line %d)", k.Value, k.Line)
			}
			seen[k.Value] = true
		}
	}
	for _, c := range n.Content {
		if err := checkYAML(c); err != nil {
			return err
		}
	}
	return nil
}
