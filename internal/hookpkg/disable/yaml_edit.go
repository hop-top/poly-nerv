// Surgical hooks.yaml mutation that preserves comments + formatting.
//
// yaml.v3's Decode → Encode round-trip discards comments and reshapes
// styling. To keep .nerv/hooks.yaml diff-friendly when toggling a
// single boolean, we operate at the Node level: parse → walk the
// hooks list → flip `disabled:` on the matching entry → re-encode the
// same Node tree.

package disable

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// setHookDisabled mutates hooks.yaml in place: for each name in names,
// finds the matching `hooks:` entry by `name:` and sets its
// `disabled:` key to value. Names that don't match any entry are
// returned in unknown. Names whose disabled state already equals value
// contribute to neither changed nor unknown — idempotent no-op.
//
// Returns:
//   - changed: names whose disabled key was actually written
//   - unknown: names with no matching hooks[].name
//   - error: yaml parse / write failure
func setHookDisabled(yamlPath string, names []string, value bool) (changed, unknown []string, err error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", yamlPath, err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", yamlPath, err)
	}

	// Root document → first content node (the top-level mapping).
	doc := documentNode(&root)
	if doc == nil {
		return nil, nil, fmt.Errorf("%s: empty document", yamlPath)
	}

	hooksList := findMapValue(doc, "hooks")
	if hooksList == nil || hooksList.Kind != yaml.SequenceNode {
		return nil, nil, fmt.Errorf("%s: no hooks: list", yamlPath)
	}

	// Index entries by name for O(N+M) lookup.
	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	seen := make(map[string]bool, len(names))

	for _, entry := range hooksList.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		nameNode := findMapValue(entry, "name")
		if nameNode == nil || nameNode.Kind != yaml.ScalarNode {
			continue
		}
		name := nameNode.Value
		if !want[name] {
			continue
		}
		seen[name] = true
		if setOrAddBool(entry, "disabled", value) {
			changed = append(changed, name)
		}
	}

	for _, n := range names {
		if !seen[n] {
			unknown = append(unknown, n)
		}
	}

	if len(changed) == 0 {
		return changed, unknown, nil
	}

	out, err := encodeNode(&root)
	if err != nil {
		return nil, nil, err
	}
	if err := atomicWrite(yamlPath, out); err != nil {
		return nil, nil, err
	}
	return changed, unknown, nil
}

// listHookNames returns the names declared in the top-level hooks: list.
func listHookNames(yamlPath string) ([]string, error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", yamlPath, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", yamlPath, err)
	}
	doc := documentNode(&root)
	if doc == nil {
		return nil, nil
	}
	hooksList := findMapValue(doc, "hooks")
	if hooksList == nil || hooksList.Kind != yaml.SequenceNode {
		return nil, nil
	}
	names := make([]string, 0, len(hooksList.Content))
	for _, entry := range hooksList.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		if nameNode := findMapValue(entry, "name"); nameNode != nil &&
			nameNode.Kind == yaml.ScalarNode {
			names = append(names, nameNode.Value)
		}
	}
	return names, nil
}

// documentNode peels the DocumentNode wrapper and returns the first
// content node (the top-level mapping). yaml.v3 wraps even single-doc
// yaml in a DocumentNode.
func documentNode(root *yaml.Node) *yaml.Node {
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil
		}
		return root.Content[0]
	}
	return root
}

// findMapValue returns the value node for key in a mapping, or nil.
// Mapping content is laid out as alternating [k0, v0, k1, v1, ...].
func findMapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setOrAddBool sets key on mapping m to a boolean scalar with value v.
// Returns true when the file content actually changed (i.e. the prior
// value differed or the key was absent). Appends key:value when
// missing; in-place updates the existing scalar otherwise.
func setOrAddBool(m *yaml.Node, key string, v bool) bool {
	if m == nil || m.Kind != yaml.MappingNode {
		return false
	}
	want := "false"
	if v {
		want = "true"
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		if k.Kind != yaml.ScalarNode || k.Value != key {
			continue
		}
		val := m.Content[i+1]
		if val.Kind == yaml.ScalarNode && val.Value == want {
			return false
		}
		val.Kind = yaml.ScalarNode
		val.Tag = "!!bool"
		val.Style = 0
		val.Value = want
		return true
	}
	// Key absent — append.
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: want},
	)
	return true
}

// removeMapKey strips key/value from a mapping. Returns true when
// removed. Used by Enable to drop the `disabled:` field entirely
// rather than setting it to false — keeps the file minimal.
func removeMapKey(m *yaml.Node, key string) bool {
	if m == nil || m.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := m.Content[i]
		if k.Kind == yaml.ScalarNode && k.Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return true
		}
	}
	return false
}

// dropHookDisabled removes the `disabled:` key from each matching
// entry. Equivalent to "enable" — semantically the same as
// setHookDisabled(..., false), but we strip the field rather than
// write false so re-enabled hooks leave no trace in the yaml.
func dropHookDisabled(yamlPath string, names []string) (changed, unknown []string, err error) {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", yamlPath, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", yamlPath, err)
	}
	doc := documentNode(&root)
	if doc == nil {
		return nil, nil, fmt.Errorf("%s: empty document", yamlPath)
	}
	hooksList := findMapValue(doc, "hooks")
	if hooksList == nil || hooksList.Kind != yaml.SequenceNode {
		return nil, nil, fmt.Errorf("%s: no hooks: list", yamlPath)
	}

	want := make(map[string]bool, len(names))
	for _, n := range names {
		want[n] = true
	}
	seen := make(map[string]bool, len(names))

	for _, entry := range hooksList.Content {
		if entry.Kind != yaml.MappingNode {
			continue
		}
		nameNode := findMapValue(entry, "name")
		if nameNode == nil || nameNode.Kind != yaml.ScalarNode {
			continue
		}
		name := nameNode.Value
		if !want[name] {
			continue
		}
		seen[name] = true
		// "Already enabled" is the natural state — either no key, or
		// key set to false. Only count as changed if we strip the key.
		disVal := findMapValue(entry, "disabled")
		if disVal == nil {
			continue
		}
		if disVal.Kind == yaml.ScalarNode && disVal.Value == "false" {
			// Strip the redundant false too — keeps file minimal.
			removeMapKey(entry, "disabled")
			changed = append(changed, name)
			continue
		}
		if removeMapKey(entry, "disabled") {
			changed = append(changed, name)
		}
	}
	for _, n := range names {
		if !seen[n] {
			unknown = append(unknown, n)
		}
	}

	if len(changed) == 0 {
		return changed, unknown, nil
	}

	out, err := encodeNode(&root)
	if err != nil {
		return nil, nil, err
	}
	if err := atomicWrite(yamlPath, out); err != nil {
		return nil, nil, err
	}
	return changed, unknown, nil
}

// encodeNode serialises a yaml.Node tree at indent=2, matching the
// project's existing emission style.
func encodeNode(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, fmt.Errorf("encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// atomicWrite writes to a temp file in the same directory then
// rename(2)s into place — avoids torn writes if the process dies
// mid-write.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hooks.yaml.*.tmp")
	if err != nil {
		return fmt.Errorf("tempfile in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		// best-effort cleanup if rename never happened
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close() // the write error is returned
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close tempfile: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod tempfile: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}
