// Package metadata contains the persistence-neutral model validation and strict
// versioned codec for aliases and scripts.
package metadata

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

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

// EncodeAliases writes a validated canonical v2 aliases document.
func EncodeAliases(aliases map[string]domain.Alias) ([]byte, error) {
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
	data, err := yaml.Marshal(aliasEnvelope{Version: CurrentVersion, Aliases: raw})
	if err != nil {
		return nil, fmt.Errorf("encode aliases: %w", err)
	}
	if len(data) > maxMetadataBytes {
		return nil, fmt.Errorf("encoded metadata is too large: maximum is %d bytes", maxMetadataBytes)
	}
	return data, nil
}

// EncodeScripts writes a validated canonical v2 scripts document.
func EncodeScripts(scripts map[string]domain.Script) ([]byte, error) {
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
	data, err := yaml.Marshal(scriptEnvelope{Version: CurrentVersion, Scripts: raw})
	if err != nil {
		return nil, fmt.Errorf("encode scripts: %w", err)
	}
	if len(data) > maxMetadataBytes {
		return nil, fmt.Errorf("encoded metadata is too large: maximum is %d bytes", maxMetadataBytes)
	}
	return data, nil
}

// DecodeAliases reads canonical v2 or a legacy aliases map. The bool is true
// only for a legacy document; decoding never publishes or otherwise changes it.
func DecodeAliases(data []byte) (map[string]domain.Alias, bool, error) {
	root, err := decodeRoot(data)
	if err != nil {
		return nil, false, err
	}
	body, legacy, err := classifyRoot(root, "aliases")
	if err != nil {
		return nil, false, err
	}
	if legacy {
		items, err := decodeAliasMap(body, true)
		return items, true, err
	}
	fields, err := mappingFields(body, "aliases envelope")
	if err != nil {
		return nil, false, err
	}
	items, err := decodeAliasMap(fields["aliases"], false)
	return items, false, err
}

// DecodeScripts reads canonical v2 or a legacy scripts map. Legacy scripts have
// an empty RecordID until a migration explicitly assigns one.
func DecodeScripts(data []byte) (map[string]domain.Script, bool, error) {
	root, err := decodeRoot(data)
	if err != nil {
		return nil, false, err
	}
	body, legacy, err := classifyRoot(root, "scripts")
	if err != nil {
		return nil, false, err
	}
	if legacy {
		items, err := decodeScriptMap(body, true)
		return items, true, err
	}
	fields, err := mappingFields(body, "scripts envelope")
	if err != nil {
		return nil, false, err
	}
	items, err := decodeScriptMap(fields["scripts"], false)
	return items, false, err
}

// DecodeScriptsReader is the bounded-reader form of DecodeScripts.
func DecodeScriptsReader(r io.Reader) (map[string]domain.Script, bool, error) {
	data, err := readBounded(r)
	if err != nil {
		return nil, false, err
	}
	return DecodeScripts(data)
}

func readBounded(r io.Reader) ([]byte, error) {
	if r == nil {
		return nil, errors.New("metadata reader is nil")
	}
	data, err := io.ReadAll(io.LimitReader(r, maxMetadataBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read metadata: %w", err)
	}
	if len(data) > maxMetadataBytes {
		return nil, fmt.Errorf("metadata file is too large: maximum is %d bytes", maxMetadataBytes)
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

func decodeAliasMap(node *yaml.Node, legacy bool) (map[string]domain.Alias, error) {
	entries, err := mappingEntries(node, "aliases")
	if err != nil {
		return nil, err
	}
	result := make(map[string]domain.Alias, len(entries))
	for name, value := range entries {
		alias, err := decodeAlias(name, value, legacy)
		if err != nil {
			return nil, err
		}
		result[name] = alias
	}
	return result, nil
}

func decodeAlias(name string, value *yaml.Node, legacy bool) (domain.Alias, error) {
	if err := ValidateName(name); err != nil {
		return domain.Alias{}, fmt.Errorf("alias %q: %w", name, err)
	}
	if legacy {
		template, err := scalarString(value)
		if err != nil {
			return domain.Alias{}, fmt.Errorf("legacy alias %q: %w", name, err)
		}
		parsed, err := parseTemplate(template)
		if err != nil {
			return domain.Alias{}, fmt.Errorf("legacy alias %q: %w", name, err)
		}
		if parsed.maxPosition == 0 && !parsed.hasArgs {
			template += " {{.args}}"
		}
		alias := domain.Alias{Name: name, Template: template}
		if err := ValidateAlias(alias); err != nil {
			return domain.Alias{}, fmt.Errorf("legacy alias %q: %w", name, err)
		}
		return alias, nil
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

func decodeScriptMap(node *yaml.Node, legacy bool) (map[string]domain.Script, error) {
	entries, err := mappingEntries(node, "scripts")
	if err != nil {
		return nil, err
	}
	result := make(map[string]domain.Script, len(entries))
	for name, value := range entries {
		script, err := decodeScript(name, value, legacy)
		if err != nil {
			return nil, err
		}
		result[name] = script
	}
	return result, nil
}

func decodeScript(name string, value *yaml.Node, legacy bool) (domain.Script, error) {
	if err := ValidateName(name); err != nil {
		return domain.Script{}, fmt.Errorf("script %q: %w", name, err)
	}
	kind := fieldScript
	if legacy {
		kind = "legacy script"
	}
	fields, err := mappingFields(value, kind)
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
	if legacy {
		if err := ValidateLegacyScript(script); err != nil {
			return domain.Script{}, fmt.Errorf("legacy script %q: %w", name, err)
		}
		return script, nil
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

func classifyRoot(root *yaml.Node, entity string) (*yaml.Node, bool, error) {
	fields, err := mappingFields(root, entity+" root")
	if err != nil {
		return nil, false, err
	}
	versionNode, hasVersion := fields["version"]
	if !hasVersion {
		return root, true, nil
	}
	if versionNode.Kind == yaml.ScalarNode && versionNode.Tag == "!!int" {
		version, err := scalarInt(versionNode)
		if err != nil {
			return nil, false, err
		}
		if version != CurrentVersion {
			return nil, false, fmt.Errorf("unsupported metadata version %d", version)
		}
		if len(fields) != 2 {
			return nil, false, fmt.Errorf("%s v2 envelope must contain only version and %s", entity, entity)
		}
		if _, ok := fields[entity]; !ok {
			return nil, false, fmt.Errorf("%s v2 envelope requires %s", entity, entity)
		}
		return root, false, nil
	}
	if isLegacyRecord(versionNode, entity) {
		return root, true, nil
	}
	return nil, false, errors.New("ambiguous metadata shape for top-level version key")
}

func isLegacyRecord(node *yaml.Node, entity string) bool {
	if entity == "aliases" {
		return node.Kind == yaml.ScalarNode && node.Tag == "!!str"
	}
	if node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == fieldScript {
			return true
		}
	}
	return false
}

func decodeRoot(data []byte) (*yaml.Node, error) {
	if len(data) > maxMetadataBytes {
		return nil, fmt.Errorf("metadata file is too large: maximum is %d bytes", maxMetadataBytes)
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
	if *count > maxYAMLNodes {
		return fmt.Errorf("metadata contains more than %d YAML nodes", maxYAMLNodes)
	}
	if depth > maxYAMLDepth {
		return fmt.Errorf("metadata YAML depth exceeds %d", maxYAMLDepth)
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
		if key.Kind != yaml.ScalarNode || key.Tag == "!!null" {
			return errors.New("mapping keys must be non-null scalars")
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
	case "legacy script":
		allowed = map[string]struct{}{fieldScript: {}, fieldDescription: {}}
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
