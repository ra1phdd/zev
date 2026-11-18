// Package yamldoc parses bounded YAML mappings independently of their metadata schema.
package yamldoc

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Structural limits apply to every metadata format.
const (
	MaxBytes = 1 << 20
	MaxNodes = 10000
	MaxDepth = 16
)

// DecodeMapping parses a single bounded YAML mapping with unique string keys.
// It enforces structural safety independently of the document's schema.
func DecodeMapping(data []byte) (map[string]*yaml.Node, error) {
	root, err := decodeRoot(data)
	if err != nil {
		return nil, err
	}
	if root.Kind != yaml.MappingNode {
		return nil, errors.New("metadata root must be a mapping")
	}
	fields := make(map[string]*yaml.Node, len(root.Content)/2)
	for i := 0; i < len(root.Content); i += 2 {
		fields[root.Content[i].Value] = root.Content[i+1]
	}
	return fields, nil
}

func decodeRoot(data []byte) (*yaml.Node, error) {
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("metadata file is too large: maximum is %d bytes", MaxBytes)
	}
	if !utf8.Valid(data) {
		return nil, errors.New("metadata file is not valid UTF-8")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("metadata document is empty")
		}
		return nil, fmt.Errorf("decode metadata: %w", err)
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, errors.New("metadata must contain exactly one YAML document")
	}
	root := document.Content[0]
	if err := inspectNode(root, 0, new(int)); err != nil {
		return nil, err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("metadata must contain exactly one YAML document")
		}
		return nil, fmt.Errorf("decode trailing metadata: %w", err)
	}
	return root, nil
}

func inspectNode(node *yaml.Node, depth int, count *int) error {
	*count++
	if *count > MaxNodes {
		return fmt.Errorf("metadata contains more than %d YAML nodes", MaxNodes)
	}
	if depth > MaxDepth {
		return fmt.Errorf("metadata YAML depth exceeds %d", MaxDepth)
	}
	if node.Kind == yaml.MappingNode && node.Tag != "!!map" || node.Kind == yaml.SequenceNode && node.Tag != "!!seq" {
		return errors.New("unexpected YAML collection tag")
	}
	if node.Alias != nil || node.Anchor != "" || (strings.HasPrefix(node.Tag, "!") && !strings.HasPrefix(node.Tag, "!!")) {
		return errors.New("YAML anchors, aliases, and custom tags are not allowed")
	}
	if err := inspectScalar(node); err != nil {
		return err
	}
	if err := inspectMapping(node); err != nil {
		return err
	}
	for _, child := range node.Content {
		if err := inspectNode(child, depth+1, count); err != nil {
			return err
		}
	}
	return nil
}

func inspectScalar(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return nil
	}
	if !utf8.ValidString(node.Value) {
		return errors.New("metadata contains invalid UTF-8")
	}
	for _, r := range node.Value {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return errors.New("metadata contains a control character")
		}
	}
	return nil
}

func inspectMapping(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	if len(node.Content)%2 != 0 {
		return errors.New("mapping has an invalid key/value shape")
	}
	seen := make(map[string]struct{}, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
			return errors.New("mapping keys must be strings")
		}
		if key.Value == "<<" {
			return errors.New("YAML merge keys are not allowed")
		}
		if _, ok := seen[key.Value]; ok {
			return fmt.Errorf("duplicate YAML key %q", key.Value)
		}
		seen[key.Value] = struct{}{}
	}
	return nil
}
