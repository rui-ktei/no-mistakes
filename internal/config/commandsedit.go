package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ProposeCommandsInFile sets the given commands.<field> entries in the
// .no-mistakes.yaml at path, creating the file and/or the commands: block when
// absent. It edits through a comment-preserving yaml.Node round-trip, so
// existing comments, key order, and unrelated content survive. A field already
// present with a non-empty value is left untouched (never overwritten), and an
// empty proposed value is skipped. It reports whether the file was changed.
func ProposeCommandsInFile(path string, updates map[CommandField]string) (bool, error) {
	pending := make(map[CommandField]string, len(updates))
	for _, field := range []CommandField{CommandFieldTest, CommandFieldLint, CommandFieldFormat} {
		if cmd := strings.TrimSpace(updates[field]); cmd != "" {
			pending[field] = updates[field]
		}
	}
	if len(pending) == 0 {
		return false, nil
	}

	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("read %s: %w", path, err)
	}

	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return false, fmt.Errorf("parse %s: %w", path, err)
		}
	}

	root := documentRoot(&doc)
	if root.Kind != yaml.MappingNode {
		return false, fmt.Errorf("%s: top-level YAML is not a mapping", path)
	}

	commands := mappingValueNode(root, "commands")
	if commands == nil {
		commands = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "commands"},
			commands,
		)
	}
	if commands.Kind != yaml.MappingNode {
		return false, fmt.Errorf("%s: commands is not a mapping", path)
	}

	changed := false
	for _, field := range []CommandField{CommandFieldTest, CommandFieldLint, CommandFieldFormat} {
		cmd, ok := pending[field]
		if !ok {
			continue
		}
		if existing := mappingValueNode(commands, string(field)); existing != nil {
			if strings.TrimSpace(existing.Value) != "" {
				continue
			}
			existing.Value = cmd
			existing.Tag = "!!str"
			existing.Style = 0
			changed = true
			continue
		}
		commands.Content = append(commands.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: string(field)},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: cmd},
		)
		changed = true
	}
	if !changed {
		return false, nil
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return false, fmt.Errorf("encode %s: %w", path, err)
	}
	if err := enc.Close(); err != nil {
		return false, fmt.Errorf("encode %s: %w", path, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// documentRoot returns the root mapping node for a parsed document, creating a
// fresh mapping (and wrapping it in the document) when the parse produced no
// usable content.
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	doc.Kind = yaml.DocumentNode
	doc.Content = []*yaml.Node{root}
	return root
}

// mappingValueNode returns the value node for key in a mapping node, or nil.
func mappingValueNode(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}
