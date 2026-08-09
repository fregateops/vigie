package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Store manages snapshot files on disk.
// Snapshots are stored as YAML files in Dir, one file per suite,
// with a top-level map from snapshot key to normalized document value.
type Store struct {
	// Dir is the directory where snapshot files are written, e.g. "tests/snapshots".
	Dir string
	// Update, when true, causes existing snapshots to be overwritten on match.
	Update bool
}

// Key returns the snapshot key for a given suite, test name, and assertion index.
func Key(suiteName, testName string, assertIdx int) string {
	return fmt.Sprintf("%s/%s[%d]", suiteName, testName, assertIdx)
}

// Match checks whether value matches the stored snapshot for key.
// If no snapshot exists for key, it writes one and returns (true, "").
// If Update is true, it overwrites the snapshot and returns (true, "").
// On mismatch, it returns (false, diff message).
func (s *Store) Match(key string, value any) (bool, string) {
	suiteName := suiteFromKey(key)
	file := s.snapshotFile(suiteName)

	// Load existing snapshots.
	snapshots, err := s.load(file)
	if err != nil && !os.IsNotExist(err) {
		return false, fmt.Sprintf("matchSnapshot: failed to load snapshot file %q: %v", file, err)
	}
	if snapshots == nil {
		snapshots = make(map[string]any)
	}

	// Normalize the incoming value.
	normalized := Normalize(value)

	existing, exists := snapshots[key]
	if !exists || s.Update {
		// Write the snapshot.
		snapshots[key] = normalized
		if writeErr := s.save(file, snapshots); writeErr != nil {
			return false, fmt.Sprintf("matchSnapshot: failed to write snapshot file %q: %v", file, writeErr)
		}
		return true, ""
	}

	// Compare.
	gotYAML, err1 := marshalSorted(normalized)
	wantYAML, err2 := marshalSorted(existing)
	if err1 != nil || err2 != nil {
		return false, fmt.Sprintf("matchSnapshot: failed to marshal for comparison: got_err=%v want_err=%v", err1, err2)
	}

	if gotYAML == wantYAML {
		return true, ""
	}

	return false, fmt.Sprintf("matchSnapshot: key %q mismatch\nwant:\n%s\ngot:\n%s", key, indent(wantYAML), indent(gotYAML))
}

// snapshotFile returns the path to the snapshot YAML file for a given suite name.
func (s *Store) snapshotFile(suiteName string) string {
	// Sanitize suite name to be a safe filename.
	safe := strings.NewReplacer("/", "_", " ", "_", ":", "_").Replace(suiteName)
	return filepath.Join(s.Dir, safe+".snap.yaml")
}

// load reads and parses the snapshot file at path.
// Returns nil, nil if path doesn't exist.
func (s *Store) load(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := yaml.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("snapshot: parse %q: %w", path, err)
	}
	return out, nil
}

// save writes snapshots map to path as YAML, creating directories as needed.
func (s *Store) save(path string, snapshots map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := marshalSnapshotFile(snapshots)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(data), 0o644)
}

// suiteFromKey extracts the suite name component from a snapshot key.
// Keys have the form "suiteName/testName[idx]".
func suiteFromKey(key string) string {
	idx := strings.Index(key, "/")
	if idx < 0 {
		return key
	}
	return key[:idx]
}

// marshalSorted marshals a value to YAML with sorted map keys for deterministic output.
func marshalSorted(v any) (string, error) {
	node, err := toYAMLNode(v)
	if err != nil {
		return "", err
	}
	out, err := yaml.Marshal(node)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// marshalSnapshotFile marshals the entire snapshot map to YAML with sorted keys.
func marshalSnapshotFile(snapshots map[string]any) (string, error) {
	keys := make([]string, 0, len(snapshots))
	for k := range snapshots {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range keys {
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
		valNode, err := toYAMLNode(snapshots[k])
		if err != nil {
			return "", err
		}
		root.Content = append(root.Content, keyNode, valNode)
	}

	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// toYAMLNode converts a Go value into a *yaml.Node for sorted-key marshaling.
func toYAMLNode(v any) (*yaml.Node, error) {
	switch val := v.(type) {
	case map[string]any:
		node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		keys := sortedKeys(val)
		for _, k := range keys {
			keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}
			valNode, err := toYAMLNode(val[k])
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, keyNode, valNode)
		}
		return node, nil
	case []any:
		node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range val {
			itemNode, err := toYAMLNode(item)
			if err != nil {
				return nil, err
			}
			node.Content = append(node.Content, itemNode)
		}
		return node, nil
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	default:
		// Let yaml handle primitives.
		data, err := yaml.Marshal(v)
		if err != nil {
			return nil, err
		}
		var n yaml.Node
		if err := yaml.Unmarshal(data, &n); err != nil {
			return nil, err
		}
		if len(n.Content) > 0 {
			return n.Content[0], nil
		}
		return &n, nil
	}
}

// indent prefixes each line of s with two spaces.
func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n")
}
