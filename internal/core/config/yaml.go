package config

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxConfigBytes = 1 << 20
	yamlString     = "!!str"
)

// Parse decodes and validates one current configuration document without I/O.
func Parse(data []byte) (*Config, error) {
	cfg, err := decodeYAML(data)
	if err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.project()
	return cfg, nil
}

func decodeYAML(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	cfg.Version = 0
	cfg.ActiveProvider = ""
	cfg.Providers = nil
	cfg.HTTPHeaders = nil
	root, err := currentDocument(data)
	if err != nil {
		return nil, err
	}
	if !validNode(root, reflect.TypeFor[Config]()) {
		return nil, errYAML
	}
	if err := root.Decode(cfg); err != nil {
		return nil, errYAML
	}
	if cfg.HTTPHeaders == nil {
		cfg.HTTPHeaders = DefaultConfig().HTTPHeaders
	}
	return cfg, nil
}

// currentDocument checks the envelope without decoding unrelated settings for help.
func currentDocument(data []byte) (*yaml.Node, error) {
	if len(data) > maxConfigBytes {
		return nil, errors.New("configuration exceeds size limit")
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil || len(doc.Content) != 1 {
		return nil, errYAML
	}
	if err := dec.Decode(new(yaml.Node)); !errors.Is(err, io.EOF) {
		return nil, errors.New("expected a single YAML configuration document")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || root.ShortTag() != "!!map" || len(root.Content) > 2*4096 {
		return nil, errYAML
	}
	var version int
	for i := 0; i < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Value != "version" {
			continue
		}
		if key.Kind != yaml.ScalarNode || key.ShortTag() != yamlString || value.Kind != yaml.ScalarNode || value.ShortTag() != "!!int" || version != 0 || value.Decode(&version) != nil || version != currentConfigVersion {
			return nil, errors.New("unsupported configuration version")
		}
	}
	if version != currentConfigVersion {
		return nil, errors.New("missing configuration version")
	}
	return root, nil
}

func encodeYAML(cfg *Config) ([]byte, error) {
	if cfg == nil {
		return nil, errors.New("config must not be nil")
	}
	c, err := cfg.forSave()
	if err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, errors.New("cannot encode YAML configuration")
	}
	if len(data) > maxConfigBytes {
		return nil, errors.New("configuration exceeds size limit")
	}
	if _, err := decodeYAML(data); err != nil {
		return nil, err
	}
	return data, nil
}

var errYAML = errors.New("invalid YAML configuration")

// yaml.v3 coerces booleans and numbers to strings and skips null fields.
// Check the resolved tags against the wire types before decoding over defaults.
func validNode(n *yaml.Node, t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String:
		return n.Kind == yaml.ScalarNode && n.ShortTag() == yamlString
	case reflect.Bool:
		return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!bool"
	case reflect.Int:
		return n.Kind == yaml.ScalarNode && n.ShortTag() == "!!int"
	case reflect.Map, reflect.Struct:
		return validMapping(n, t)
	default:
		return false
	}
}

var fields = map[reflect.Type]map[string]reflect.Type{}

func init() {
	for _, t := range []reflect.Type{reflect.TypeFor[Config](), reflect.TypeFor[ProviderConfig]()} {
		fields[t] = make(map[string]reflect.Type)
		for f := range t.Fields() {
			name, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
			if name != "" && name != "-" {
				fields[t][name] = f.Type
			}
		}
	}
}

func validMapping(n *yaml.Node, t reflect.Type) bool {
	// Bound the quadratic duplicate-key check in yaml.v3's typed decoder.
	if n.Kind != yaml.MappingNode || n.ShortTag() != "!!map" || len(n.Content) > 2*4096 {
		return false
	}
	seen := make(map[string]bool, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		k := n.Content[i]
		if k.Kind != yaml.ScalarNode || k.ShortTag() != yamlString || seen[k.Value] {
			return false
		}
		seen[k.Value] = true
		var child reflect.Type
		if t.Kind() == reflect.Map {
			child = t.Elem()
		} else {
			child = fields[t][k.Value]
		}
		if child == nil || !validNode(n.Content[i+1], child) {
			return false
		}
	}
	return true
}
