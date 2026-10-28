package opencode

import (
	"encoding/json"
	"testing"
)

// MergeConfig output is written to ConfigPaths()[0] (opencode.json) by
// the installer, so it must be that file merged, not the TS plugin.
func TestAdapter_MergeConfig_MergesOpencodeJSON(t *testing.T) {
	a := &Adapter{NervdPath: "/usr/local/bin/nervd"}
	existing := []byte(`{"model":"claude-4","nerv":{"note":"kept"}}`)

	merged, err := a.MergeConfig(existing)
	if err != nil {
		t.Fatalf("MergeConfig: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("merged output not JSON: %v\n%s", err, merged)
	}
	if got["model"] != "claude-4" {
		t.Errorf("model = %v, want claude-4", got["model"])
	}
	nerv, _ := got["nerv"].(map[string]any)
	if nerv["installed"] != true {
		t.Errorf("nerv.installed = %v, want true", nerv["installed"])
	}
	if nerv["note"] != "kept" {
		t.Errorf("nerv.note = %v, want kept", nerv["note"])
	}
}

func TestAdapter_MergeConfig_Idempotent(t *testing.T) {
	a := &Adapter{NervdPath: "/usr/local/bin/nervd"}
	first, err := a.MergeConfig(nil)
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	second, err := a.MergeConfig(first)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("not idempotent:\n%s\n---\n%s", first, second)
	}
}

// The marker claims the bridge is installable; without a nervd path no
// plugin can be generated, so the merge must refuse.
func TestAdapter_MergeConfig_RequiresNervdPath(t *testing.T) {
	if _, err := (&Adapter{}).MergeConfig(nil); err == nil {
		t.Error("MergeConfig with empty NervdPath: want error, got nil")
	}
}
