// Package metadata contains the persistence-neutral model validation and strict
// versioned codec for aliases and scripts.
package metadata

import (
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/ra1phdd/zev/internal/core/metadata/yamldoc"

	"github.com/ra1phdd/zev/internal/core/domain"
	"gopkg.in/yaml.v3"
)

type argumentYAML struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Required    *bool  `yaml:"required"`
}

type aliasYAML struct {
	Template    string         `yaml:"template"`
	Description string         `yaml:"description"`
	Arguments   []argumentYAML `yaml:"arguments"`
}

const (
	fieldDescription = "description"
	fieldScript      = "script"
)

type scriptYAML struct {
	RecordID    string         `yaml:"recordID"` //nolint:tagliatelle // Canonical YAML contract spells this field recordID.
	Script      string         `yaml:"script"`
	Description string         `yaml:"description"`
	Arguments   []argumentYAML `yaml:"arguments"`
}

type aliasEnvelope struct {
	Version int                  `yaml:"version"`
	Aliases map[string]aliasYAML `yaml:"aliases"`
}

type scriptEnvelope struct {
	Version int                   `yaml:"version"`
	Scripts map[string]scriptYAML `yaml:"scripts"`
}

// EncodeAliasesV2 writes a validated canonical v2 aliases document.
func EncodeAliasesV2(aliases map[string]domain.Alias) ([]byte, error) {
	raw := make(map[string]aliasYAML, len(aliases))
	for name, alias := range aliases {
		if alias.Name == "" {
			alias.Name = name
		}
		if alias.Name != name {
			return nil, fmt.Errorf("alias key %q does not match record name %q", name, alias.Name)
		}
		if err := ValidateAlias(alias); err != nil {
			return nil, fmt.Errorf("invalid alias %q: %w", name, err)
		}
		raw[name] = aliasYAML{Template: alias.Template, Description: alias.Description, Arguments: encodeArguments(alias.Arguments)}
	}
	data, err := yaml.Marshal(aliasEnvelope{Version: 2, Aliases: raw})
	if err != nil {
		return nil, fmt.Errorf("encode aliases: %w", err)
	}
	if len(data) > yamldoc.MaxBytes {
		return nil, fmt.Errorf("encoded metadata is too large: maximum is %d bytes", yamldoc.MaxBytes)
	}
	return data, nil
}

// EncodeScriptsV2 writes a validated canonical v2 scripts document.
func EncodeScriptsV2(scripts map[string]domain.Script) ([]byte, error) {
	raw := make(map[string]scriptYAML, len(scripts))
	for name, script := range scripts {
		if script.Name == "" {
			script.Name = name
		}
		if script.Name != name {
			return nil, fmt.Errorf("script key %q does not match record name %q", name, script.Name)
		}
		if err := ValidateScript(script); err != nil {
			return nil, fmt.Errorf("invalid script %q: %w", name, err)
		}
		raw[name] = scriptYAML{RecordID: script.RecordID, Script: script.ScriptFile, Description: script.Description, Arguments: encodeArguments(script.Arguments)}
	}
	data, err := yaml.Marshal(scriptEnvelope{Version: 2, Scripts: raw})
	if err != nil {
		return nil, fmt.Errorf("encode scripts: %w", err)
	}
	if len(data) > yamldoc.MaxBytes {
		return nil, fmt.Errorf("encoded metadata is too large: maximum is %d bytes", yamldoc.MaxBytes)
	}
	return data, nil
}

// DecodeAliases reads the current v2 aliases envelope.
func DecodeAliases(data []byte) (map[string]domain.Alias, error) {
	body, err := envelope(data, "aliases")
	if err != nil {
		return nil, err
	}
	return decodeAliasMap(body)
}

// DecodeScripts reads the current v2 scripts envelope.
func DecodeScripts(data []byte) (map[string]domain.Script, error) {
	body, err := envelope(data, "scripts")
	if err != nil {
		return nil, err
	}
	return decodeScriptMap(body)
}

// ReadBounded reads a metadata payload within the codec size limit.
func ReadBounded(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, errors.New("metadata reader is nil")
	}
	data, err := io.ReadAll(io.LimitReader(r, yamldoc.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	if len(data) > yamldoc.MaxBytes {
		return nil, fmt.Errorf("metadata file is too large: maximum is %d bytes", yamldoc.MaxBytes)
	}
	return data, nil
}

func encodeArguments(arguments []domain.Argument) []argumentYAML {
	result := make([]argumentYAML, len(arguments))
	for i, argument := range arguments {
		required := argument.Required
		result[i] = argumentYAML{Name: argument.Name, Description: argument.Description, Required: &required}
	}
	return result
}

func decodeArguments(node *yaml.Node) ([]domain.Argument, error) {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil, errors.New("arguments must be a sequence")
	}
	result := make([]domain.Argument, len(node.Content))
	for i, item := range node.Content {
		fields, err := mappingFields(item, "argument")
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", i+1, err)
		}
		nameNode, ok := fields["name"]
		if !ok {
			return nil, fmt.Errorf("argument %d: name is required", i+1)
		}
		name, err := scalarString(nameNode)
		if err != nil {
			return nil, fmt.Errorf("argument %d name: %w", i+1, err)
		}
		argument := domain.Argument{Name: name, Required: true}
		if description, ok := fields[fieldDescription]; ok {
			argument.Description, err = scalarString(description)
			if err != nil {
				return nil, fmt.Errorf("argument %d description: %w", i+1, err)
			}
		}
		if required, ok := fields["required"]; ok {
			argument.Required, err = scalarBool(required)
			if err != nil {
				return nil, fmt.Errorf("argument %d required: %w", i+1, err)
			}
		}
		result[i] = argument
	}
	if err := ValidateArguments(result); err != nil {
		return nil, err
	}
	return result, nil
}

func decodeAliasMap(node *yaml.Node) (map[string]domain.Alias, error) {
	entries, err := mappingEntries(node, "aliases")
	if err != nil {
		return nil, err
	}
	result := make(map[string]domain.Alias, len(entries))
	for name, value := range entries {
		alias, err := decodeAlias(name, value)
		if err != nil {
			return nil, err
		}
		result[name] = alias
	}
	return result, nil
}

func decodeAlias(name string, value *yaml.Node) (domain.Alias, error) {
	if err := ValidateName(name); err != nil {
		return domain.Alias{}, fmt.Errorf("alias %q: %w", name, err)
	}
	fields, err := mappingFields(value, "alias")
	if err != nil {
		return domain.Alias{}, fmt.Errorf("alias %q: %w", name, err)
	}
	templateNode, ok := fields["template"]
	if !ok {
		return domain.Alias{}, fmt.Errorf("alias %q: template is required", name)
	}
	template, err := scalarString(templateNode)
	if err != nil {
		return domain.Alias{}, fmt.Errorf("alias %q template: %w", name, err)
	}
	alias := domain.Alias{Name: name, Template: template}
	if err := decodeDescription(fields, &alias.Description); err != nil {
		return domain.Alias{}, fmt.Errorf("alias %q description: %w", name, err)
	}
	if arguments, ok := fields["arguments"]; ok {
		alias.Arguments, err = decodeArguments(arguments)
		if err != nil {
			return domain.Alias{}, fmt.Errorf("alias %q arguments: %w", name, err)
		}
	}
	if err := ValidateAlias(alias); err != nil {
		return domain.Alias{}, fmt.Errorf("alias %q: %w", name, err)
	}
	return alias, nil
}

func decodeDescription(fields map[string]*yaml.Node, destination *string) error {
	description, ok := fields[fieldDescription]
	if !ok {
		return nil
	}
	value, err := scalarString(description)
	if err != nil {
		return err
	}
	*destination = value
	return nil
}

func decodeScriptMap(node *yaml.Node) (map[string]domain.Script, error) {
	entries, err := mappingEntries(node, "scripts")
	if err != nil {
		return nil, err
	}
	result := make(map[string]domain.Script, len(entries))
	for name, value := range entries {
		script, err := decodeScript(name, value)
		if err != nil {
			return nil, err
		}
		result[name] = script
	}
	return result, nil
}

func decodeScript(name string, value *yaml.Node) (domain.Script, error) {
	if err := ValidateName(name); err != nil {
		return domain.Script{}, fmt.Errorf("script %q: %w", name, err)
	}
	fields, err := mappingFields(value, fieldScript)
	if err != nil {
		return domain.Script{}, fmt.Errorf("script %q: %w", name, err)
	}
	scriptNode, ok := fields[fieldScript]
	if !ok {
		return domain.Script{}, fmt.Errorf("script %q: script is required", name)
	}
	scriptFile, err := scalarString(scriptNode)
	if err != nil {
		return domain.Script{}, fmt.Errorf("script %q path: %w", name, err)
	}
	script := domain.Script{Name: name, ScriptFile: scriptFile}
	if err := decodeDescription(fields, &script.Description); err != nil {
		return domain.Script{}, fmt.Errorf("script %q description: %w", name, err)
	}
	if err := decodeCanonicalScriptFields(fields, &script); err != nil {
		return domain.Script{}, fmt.Errorf("script %q: %w", name, err)
	}
	if err := ValidateScript(script); err != nil {
		return domain.Script{}, fmt.Errorf("script %q: %w", name, err)
	}
	return script, nil
}

func decodeCanonicalScriptFields(fields map[string]*yaml.Node, script *domain.Script) error {
	recordID, ok := fields["recordID"]
	if !ok {
		return errors.New("recordID is required")
	}
	value, err := scalarString(recordID)
	if err != nil {
		return fmt.Errorf("recordID: %w", err)
	}
	script.RecordID = value
	if arguments, ok := fields["arguments"]; ok {
		script.Arguments, err = decodeArguments(arguments)
		if err != nil {
			return fmt.Errorf("arguments: %w", err)
		}
	}
	return nil
}

func envelope(data []byte, entity string) (*yaml.Node, error) {
	fields, err := yamldoc.DecodeMapping(data)
	if err != nil {
		return nil, fmt.Errorf("decode %s envelope: %w", entity, err)
	}
	version, err := scalarInt(fields["version"])
	if err != nil {
		return nil, err
	}
	if version != 2 {
		return nil, fmt.Errorf("unsupported metadata version %d", version)
	}
	if len(fields) != 2 || fields[entity] == nil {
		return nil, fmt.Errorf("%s v2 envelope must contain only version and %s", entity, entity)
	}
	return fields[entity], nil
}

func mappingFields(node *yaml.Node, kind string) (map[string]*yaml.Node, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", kind)
	}
	allowed := map[string]struct{}(nil)
	switch kind {
	case "alias":
		allowed = map[string]struct{}{"template": {}, fieldDescription: {}, "arguments": {}}
	case fieldScript:
		allowed = map[string]struct{}{"recordID": {}, fieldScript: {}, fieldDescription: {}, "arguments": {}}
	case "argument":
		allowed = map[string]struct{}{"name": {}, fieldDescription: {}, "required": {}}
	}
	result := make(map[string]*yaml.Node, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("%s keys must be scalars", kind)
		}
		name := key.Value
		if allowed != nil {
			if _, ok := allowed[name]; !ok {
				return nil, fmt.Errorf("unknown field %q", name)
			}
		}
		result[name] = node.Content[i+1]
	}
	return result, nil
}

func mappingEntries(node *yaml.Node, kind string) (map[string]*yaml.Node, error) {
	return mappingFields(node, kind)
}

func scalarString(node *yaml.Node) (string, error) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", errors.New("expected a non-null string")
	}
	return node.Value, nil
}

func scalarInt(node *yaml.Node) (int, error) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
		return 0, errors.New("version must be an integer")
	}
	value, err := strconv.Atoi(node.Value)
	if err != nil {
		return 0, fmt.Errorf("decode version: %w", err)
	}
	return value, nil
}

func scalarBool(node *yaml.Node) (bool, error) {
	if node == nil || node.Kind != yaml.ScalarNode || node.Tag != "!!bool" {
		return false, errors.New("required must be a boolean")
	}
	value, err := strconv.ParseBool(node.Value)
	if err != nil {
		return false, fmt.Errorf("decode boolean: %w", err)
	}
	return value, nil
}
