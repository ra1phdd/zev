package scripts

import (
	"context"
	"fmt"

	"github.com/ra1phdd/zev/internal/core/metadata/yamldoc"

	"github.com/google/uuid"
	"github.com/ra1phdd/zev/internal/core/domain"
	"github.com/ra1phdd/zev/internal/core/metadata"
	"gopkg.in/yaml.v3"
)

func toV2(_ context.Context, source []byte) ([]byte, error) {
	entries, err := yamldoc.DecodeMapping(source)
	if err != nil {
		return nil, fmt.Errorf("decode legacy scripts: %w", err)
	}
	records := make(map[string]domain.Script, len(entries))
	for name, node := range entries {
		script, err := scriptRecord(name, node)
		if err != nil {
			return nil, err
		}
		records[name] = script
	}
	data, err := metadata.EncodeScriptsV2(records)
	if err != nil {
		return nil, fmt.Errorf("encode v2 scripts: %w", err)
	}
	return data, nil
}

func scriptRecord(name string, node *yaml.Node) (domain.Script, error) {
	if node.Kind != yaml.MappingNode {
		return domain.Script{}, fmt.Errorf("script %q must be a mapping", name)
	}
	record := domain.Script{Name: name, RecordID: uuid.NewString()}
	for i := 0; i < len(node.Content); i += 2 {
		key, val := node.Content[i].Value, node.Content[i+1]
		if val.Kind != yaml.ScalarNode || val.Tag != "!!str" {
			return domain.Script{}, fmt.Errorf("script %q: %s must be a string", name, key)
		}
		switch key {
		case "script":
			record.ScriptFile = val.Value
		case "description":
			record.Description = val.Value
		default:
			return domain.Script{}, fmt.Errorf("script %q: unknown field %q", name, key)
		}
	}
	return record, nil
}
