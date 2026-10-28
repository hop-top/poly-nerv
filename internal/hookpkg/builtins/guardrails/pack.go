package guardrails

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Pack is one loaded rulepack (spec/rulepacks/<name>.yaml).
type Pack struct {
	Name        string `yaml:"name"`
	Version     int    `yaml:"version"`
	Description string `yaml:"description"`
	AppliesTo   struct {
		Tool string `yaml:"tool"`
	} `yaml:"applies_to"`
	Rules []Rule `yaml:"rules"`
}

// Rule is one entry in a Pack.Rules slice.
type Rule struct {
	ID             string            `yaml:"id"`
	Description    string            `yaml:"description"`
	Severity       string            `yaml:"severity"`
	Pattern        string            `yaml:"pattern"`
	DefaultMode    string            `yaml:"default_mode"`
	Rewrites       []RewriteStrategy `yaml:"rewrites,omitempty"`
	Destructive    bool              `yaml:"destructive,omitempty"`
	BackupStrategy string            `yaml:"backup_strategy,omitempty"`
	Reason         string            `yaml:"reason"`

	// compiled is the cached regexp; set by Compile.
	compiled *regexp.Regexp
}

// RewriteStrategy is one entry in Rule.Rewrites. Only one of the
// strategy-specific fields is set per entry; Strategy names the
// active one.
type RewriteStrategy struct {
	Strategy string   `yaml:"strategy"`
	Flag     string   `yaml:"flag,omitempty"`
	From     string   `yaml:"from,omitempty"`
	To       string   `yaml:"to,omitempty"`
	Args     []string `yaml:"args,omitempty"`
}

// LoadPacks reads every *.yaml in dir and returns the parsed +
// regex-compiled pack list, sorted by pack name. Files are loaded in
// lexical order so first-match-wins is deterministic.
func LoadPacks(dir string) ([]Pack, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("guardrails: read rulepacks dir: %w", err)
	}

	var packs []Pack
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		p, err := loadPack(path)
		if err != nil {
			return nil, fmt.Errorf("guardrails: load %s: %w", path, err)
		}
		packs = append(packs, p)
	}

	sort.Slice(packs, func(i, j int) bool { return packs[i].Name < packs[j].Name })
	return packs, nil
}

func loadPack(path string) (Pack, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Pack{}, err
	}
	var p Pack
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Pack{}, fmt.Errorf("yaml: %w", err)
	}
	if err := p.Compile(); err != nil {
		return Pack{}, err
	}
	if err := p.Validate(); err != nil {
		return Pack{}, err
	}
	return p, nil
}

// Compile pre-compiles every rule's regex. Bad patterns surface at
// daemon startup, not first-event time.
func (p *Pack) Compile() error {
	for i := range p.Rules {
		r := &p.Rules[i]
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return fmt.Errorf("rule %s: bad pattern %q: %w", r.ID, r.Pattern, err)
		}
		r.compiled = re
	}
	return nil
}

// Validate enforces invariants the JSON schema cannot:
//   - destructive=true rules MUST set backup_strategy
//   - rewrite-mode rules MUST declare at least one rewrite
//   - rule ids are unique within the pack
func (p *Pack) Validate() error {
	seen := make(map[string]bool, len(p.Rules))
	for _, r := range p.Rules {
		if seen[r.ID] {
			return fmt.Errorf("duplicate rule id %q in pack %s", r.ID, p.Name)
		}
		seen[r.ID] = true

		if r.Destructive && r.BackupStrategy == "" {
			return fmt.Errorf("rule %s: destructive=true requires backup_strategy", r.ID)
		}
		if r.DefaultMode == "rewrite" && len(r.Rewrites) == 0 {
			return fmt.Errorf("rule %s: default_mode=rewrite requires at least one rewrite", r.ID)
		}
	}
	return nil
}

// Regex returns the compiled pattern. Helper used by Match.
func (r *Rule) Regex() *regexp.Regexp { return r.compiled }
