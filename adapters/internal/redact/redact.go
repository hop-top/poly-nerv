// Package redact provides secret redaction for nerv envelopes.
//
// Patterns derived from spec/redaction.yaml. Applied in definition
// order; first match wins per span. Runs in adapter before envelope
// crosses to nervd.
//
// TODO: load patterns from embedded spec/redaction.yaml instead of
// hardcoding. Requires embed path accessible from this package.
package redact

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"hop.top/nerv/internal/envelope"
)

// DefaultTimeout is the per-envelope redaction budget.
const DefaultTimeout = 100 * time.Millisecond

// Pattern represents a single redaction rule.
type Pattern struct {
	Name        string
	Regex       *regexp.Regexp
	Replacement string
	Scope       string // "payload" or "payload.content"
}

// Engine holds compiled redaction patterns.
type Engine struct {
	patterns []Pattern
	timeout  time.Duration
}

// New creates a redaction engine with all spec patterns compiled.
func New() *Engine {
	return &Engine{
		patterns: defaultPatterns(),
		timeout:  DefaultTimeout,
	}
}

// NewWithTimeout creates an engine with a custom timeout.
func NewWithTimeout(d time.Duration) *Engine {
	return &Engine{
		patterns: defaultPatterns(),
		timeout:  d,
	}
}

// Redact applies all patterns to the input string.
// Returns redacted string. On timeout: returns error (fail-safe).
func (e *Engine) Redact(input string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	result := input
	for _, p := range e.patterns {
		if p.Scope == "payload.content" {
			continue // handled by HashContent
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("redact: timeout after %v", e.timeout)
		default:
		}
		result = p.Regex.ReplaceAllString(result, p.Replacement)
	}
	return result, nil
}

// RedactEnvelope redacts the payload field of an envelope.
// If fullPayload is true (handler requires full_payload), skip entirely.
func (e *Engine) RedactEnvelope(env *envelope.Envelope, fullPayload bool) error {
	if fullPayload {
		return nil
	}

	raw := string(env.Payload)
	redacted, err := e.Redact(raw)
	if err != nil {
		return err
	}
	env.Payload = json.RawMessage(redacted)
	return nil
}

// HashContent replaces content with a {sha256, bytes} summary.
func HashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf(`{"sha256":"%x","bytes":%d}`, h, len(content))
}

// defaultPatterns returns compiled patterns from spec/redaction.yaml.
func defaultPatterns() []Pattern {
	defs := []struct {
		name        string
		regex       string
		replacement string
		scope       string
	}{
		{
			name:        "env_var_expansion",
			regex:       `\$\{?[A-Z_][A-Z0-9_]*\}?`,
			replacement: `${REDACTED:$0}`,
			scope:       "payload",
		},
		{
			name:        "dotenv_content",
			regex:       `(?m)^[A-Z_][A-Z0-9_]*=.+$`,
			replacement: `${REDACTED:env_line}`,
			scope:       "payload",
		},
		{
			name:        "bearer_token",
			regex:       `Bearer\s+[A-Za-z0-9\-._~+/]+=*`,
			replacement: `Bearer ${REDACTED:token}`,
			scope:       "payload",
		},
		{
			name:        "api_key_pattern",
			regex:       `(?:sk|pk|api|key|token|secret|password|auth)[_-]?[A-Za-z0-9]{20,}`,
			replacement: `${REDACTED:api_key}`,
			scope:       "payload",
		},
		{
			name:        "jwt_token",
			regex:       `eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`,
			replacement: `${REDACTED:jwt}`,
			scope:       "payload",
		},
		{
			name:        "private_key_block",
			regex:       `(?s)-----BEGIN (?:RSA |EC |DSA )?PRIVATE KEY-----.*?-----END (?:RSA |EC |DSA )?PRIVATE KEY-----`,
			replacement: `${REDACTED:private_key}`,
			scope:       "payload",
		},
		{
			name:        "aws_access_key",
			regex:       `AKIA[0-9A-Z]{16}`,
			replacement: `${REDACTED:aws_access_key}`,
			scope:       "payload",
		},
		{
			name:        "aws_secret_key",
			regex:       `aws_secret_access_key\s*=\s*[A-Za-z0-9/+=]{40}`,
			replacement: `aws_secret_access_key=${REDACTED:aws_secret_key}`,
			scope:       "payload",
		},
		{
			name:        "github_token",
			regex:       `(?:ghp|gho|ghs|ghr)_[A-Za-z0-9_]{36,}|github_pat_[A-Za-z0-9_]{22,}`,
			replacement: `${REDACTED:github_token}`,
			scope:       "payload",
		},
		{
			name:        "slack_token",
			regex:       `xox[bpea]-[0-9A-Za-z\-]{24,}`,
			replacement: `${REDACTED:slack_token}`,
			scope:       "payload",
		},
		{
			name:        "connection_string",
			regex:       `(?:mongodb(?:\+srv)?|postgres(?:ql)?|mysql|redis|amqp|mssql)://[^@\s/"'` + "`" + `]+(?::[^@\s/"'` + "`" + `]*)?@[^\s"'` + "`" + `]+`,
			replacement: `${REDACTED:connection_string}`,
			scope:       "payload",
		},
		{
			name:        "base64_secret",
			regex:       `(?:secret|key|password|token|credential)\s*[:=]\s*[A-Za-z0-9+/]{40,}={0,2}`,
			replacement: `${REDACTED:base64_secret}`,
			scope:       "payload",
		},
		{
			name:        "write_tool_content",
			regex:       `$^`, // never matches; handled via HashContent
			replacement: ``,
			scope:       "payload.content",
		},
	}

	patterns := make([]Pattern, 0, len(defs))
	for _, d := range defs {
		re, err := regexp.Compile(d.regex)
		if err != nil {
			// Spec says: skip pattern, log error at init.
			continue
		}
		patterns = append(patterns, Pattern{
			Name:        d.name,
			Regex:       re,
			Replacement: d.replacement,
			Scope:       d.scope,
		})
	}
	return patterns
}
