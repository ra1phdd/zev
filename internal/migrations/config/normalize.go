package config

import (
	"bytes"
	"errors"

	"gopkg.in/yaml.v3"
)

func normalizeLegacyYAML(data []byte) ([]byte, bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil || len(document.Content) != 1 {
		return nil, false, errors.New("invalid legacy YAML configuration")
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, false, errors.New("invalid legacy YAML configuration")
	}
	changed := false
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "security" || root.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		security := root.Content[i+1]
		for j := 0; j+1 < len(security.Content); j += 2 {
			if security.Content[j].Value != "sandbox" {
				continue
			}
			legacy := security.Content[j+1]
			if legacy.Kind != yaml.MappingNode || len(legacy.Content) != 2 || legacy.Content[0].Value != "mode" || legacy.Content[0].ShortTag() != "!!str" || legacy.Content[1].Kind != yaml.ScalarNode || legacy.Content[1].ShortTag() != "!!str" || legacy.Content[1].Value != "off" {
				return nil, false, errors.New("unsupported legacy configuration setting")
			}
			security.Content = append(security.Content[:j], security.Content[j+2:]...)
			changed = true
			break
		}
	}
	if !changed {
		return data, false, nil
	}
	var output bytes.Buffer
	if err := yaml.NewEncoder(&output).Encode(root); err != nil {
		return nil, false, errors.New("cannot normalize legacy YAML configuration")
	}
	return output.Bytes(), true, nil
}
