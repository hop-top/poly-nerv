// Package dispatch matches envelopes to hooks using the pre-compiled
// CachedConfig index. Regex patterns are lazily compiled and cached
// per-process for the lifetime of the Matcher.
package dispatch

import (
	"encoding/json"
	"log"
	"regexp"
	"sync"

	"hop.top/nerv/internal/config"
	"hop.top/nerv/internal/envelope"
)

// MatchResult contains hooks that match a given envelope.
type MatchResult struct {
	Hooks []config.HookEntry
}

// Matcher holds compiled regexes and capability state.
type Matcher struct {
	regexCache map[string]*regexp.Regexp
	mu         sync.RWMutex
}

// NewMatcher creates a matcher with an empty regex cache.
func NewMatcher() *Matcher {
	return &Matcher{
		regexCache: make(map[string]*regexp.Regexp),
	}
}

// toolPayload is used to extract tool_name from the envelope payload.
type toolPayload struct {
	ToolName string `json:"tool_name"`
}

// Match returns hooks from cfg that match env.
//
// For each hook registered under the envelope's event:
//  1. Skip if require_capability is set (capability checking wired later).
//  2. If match regex is set, compile (cached) and test against tool_name
//     extracted from the envelope payload. Invalid regex → skip + warn.
//  3. No match field → hook passes unconditionally.
//
// Returned hooks preserve hooks.yaml declaration order.
func (m *Matcher) Match(cfg *config.CachedConfig, env *envelope.Envelope) *MatchResult {
	result := &MatchResult{}

	bucket, ok := cfg.ByEvent[env.Event]
	if !ok {
		return result
	}

	// Extract tool_name once for the whole bucket.
	toolName := m.extractToolName(env)

	for _, h := range bucket {
		// Step 1: capability gate — always skip if set (no cap map yet).
		if h.RequireCapability != "" {
			continue
		}

		// Step 2: regex filter on tool_name.
		if h.Match != "" {
			re, err := m.compile(h.Match)
			if err != nil {
				log.Printf("dispatch: skip hook %q: bad regex %q: %v",
					h.Name, h.Match, err)
				continue
			}
			if !re.MatchString(toolName) {
				continue
			}
		}

		// Step 3: passed all gates.
		result.Hooks = append(result.Hooks, h)
	}

	return result
}

// RegexCacheLen returns the number of compiled regexes. Exported for
// testing only.
func (m *Matcher) RegexCacheLen() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.regexCache)
}

// compile returns a cached *regexp.Regexp, compiling on first access.
func (m *Matcher) compile(pattern string) (*regexp.Regexp, error) {
	m.mu.RLock()
	if re, ok := m.regexCache[pattern]; ok {
		m.mu.RUnlock()
		return re, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after upgrade.
	if re, ok := m.regexCache[pattern]; ok {
		return re, nil
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	m.regexCache[pattern] = re
	return re, nil
}

// extractToolName unmarshals the payload to pull out tool_name.
// Returns "" if payload is nil or tool_name is absent.
func (m *Matcher) extractToolName(env *envelope.Envelope) string {
	if len(env.Payload) == 0 {
		return ""
	}
	var tp toolPayload
	if err := json.Unmarshal(env.Payload, &tp); err != nil {
		return ""
	}
	return tp.ToolName
}
