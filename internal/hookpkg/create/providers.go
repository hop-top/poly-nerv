package create

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hop.top/kit/go/ai/llm"
)

// NewGenerator resolves an llm.Provider from the supplied URI (or, when
// empty, from $LLM_PROVIDER / the user's llm.yaml default) and returns
// a Generator that completes via the resolved provider.
//
// providerURI accepts the same syntax kit/ai/llm understands —
// scheme://[host:port/]model[?param=val] — e.g.
//
//	anthropic://claude-sonnet-4-5
//	openai://gpt-4o-mini
//	ollama://localhost:11434/llama3
//
// API keys MUST come from the provider's documented env var
// (ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.) or the llm.yaml block —
// never from URI query params. Query params land in process listings,
// shell history, and error output; the user-facing error path here
// sanitises them via sanitizeURIForLog, but any caller that constructs
// the URI from a credential leaks it before NewGenerator sees it.
//
// When empty, kit's LoadConfig walks: env var LLM_PROVIDER → llm.yaml
// `default:` → returns an error if neither is set.
//
// The adapter packages (anthropic, openai, ollama, …) must be imported
// for side-effect registration BEFORE calling NewGenerator. cmd/nerv
// anonymously imports them in main.go; tests don't need them because
// they construct a Generator directly with a mock llm.Completer.
func NewGenerator(providerURI string) (Generator, error) {
	cfg, err := llm.LoadConfig(providerURI)
	if err != nil {
		return nil, fmt.Errorf("create: resolve provider %q: %w",
			sanitizeURIForLog(providerURI), err)
	}

	uriString := providerURI
	if uriString == "" {
		uriString = renderURI(cfg.URI)
	}

	provider, err := llm.Resolve(uriString)
	if err != nil {
		return nil, fmt.Errorf("create: resolve %q: %w",
			sanitizeURIForLog(uriString), err)
	}

	completer, ok := provider.(llm.Completer)
	if !ok {
		_ = provider.Close()
		return nil, fmt.Errorf(
			"create: provider %q does not implement llm.Completer",
			cfg.URI.Scheme,
		)
	}

	return &kitLLMGenerator{c: completer, model: cfg.URI.Model}, nil
}

// sanitizeURIForLog returns a redacted form of a provider URI safe to
// embed in user-facing error messages and logs. It drops query params
// (api keys), userinfo (user:pass@), and fragments; keeps scheme +
// host + path so the operator can still recognise which provider
// failed. An empty input renders as "(default)".
//
// Best-effort: a malformed URI is returned with a redaction marker on
// any "?" or "@" splits so a partial parse can't leak the tail.
func sanitizeURIForLog(raw string) string {
	if raw == "" {
		return "(default)"
	}
	out := raw
	if i := strings.Index(out, "?"); i >= 0 {
		out = out[:i]
	}
	if i := strings.Index(out, "#"); i >= 0 {
		out = out[:i]
	}
	// Strip userinfo (scheme://user:pass@host/...). Only the authority
	// segment can carry an "@", so we look for "://" first.
	if i := strings.Index(out, "://"); i >= 0 {
		head := out[:i+3]
		tail := out[i+3:]
		if at := strings.Index(tail, "@"); at >= 0 {
			tail = tail[at+1:]
		}
		out = head + tail
	}
	return out
}

// renderURI reconstructs a URI string from a parsed llm.URI. Used when
// LoadConfig resolved a default from env/config so Resolve gets the
// same string.
func renderURI(u llm.URI) string {
	var sb strings.Builder
	sb.WriteString(u.Scheme)
	sb.WriteString("://")
	if u.Host != "" {
		sb.WriteString(u.Host)
		sb.WriteString("/")
	}
	sb.WriteString(u.Model)
	if len(u.Params) > 0 {
		sb.WriteString("?")
		first := true
		for k, v := range u.Params {
			if !first {
				sb.WriteString("&")
			}
			first = false
			sb.WriteString(k)
			sb.WriteString("=")
			sb.WriteString(v)
		}
	}
	return sb.String()
}

// NewGeneratorFromCompleter wraps a raw llm.Completer (typically a mock
// or a Provider already resolved by the caller) into a Generator. The
// model string is forwarded into each request.
//
// Exported so tests can inject deterministic completers without going
// through the registry.
func NewGeneratorFromCompleter(c llm.Completer, model string) Generator {
	return &kitLLMGenerator{c: c, model: model}
}

// kitLLMGenerator is the production Generator. It renders the embedded
// prompt template and calls llm.Completer.Complete, parsing the strict
// JSON response into a Result.
type kitLLMGenerator struct {
	c     llm.Completer
	model string
}

// Generate implements Generator.
func (g *kitLLMGenerator) Generate(
	ctx context.Context, intent, lang string, eventsOverride []string,
) (*Result, error) {
	if strings.TrimSpace(intent) == "" {
		return nil, ErrEmptyIntent
	}
	if err := ValidateLang(lang); err != nil {
		return nil, err
	}

	prompt := renderPrompt(intent, lang, eventsOverride)

	req := llm.Request{
		Model: g.model,
		Messages: []llm.Message{
			{Role: "user", Content: prompt},
		},
		// Conservative cap — the handler script + JSON wrapper is rarely
		// large; clamping protects against runaway generations.
		MaxTokens: 4096,
	}

	resp, err := g.c.Complete(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create: llm.Complete: %w", err)
	}

	res, err := parseResponse(resp.Content)
	if err != nil {
		return nil, fmt.Errorf("create: parse model response: %w", err)
	}
	res.Lang = lang
	if len(eventsOverride) > 0 {
		// Honour the override even if the model ignored it.
		res.Events = append([]string(nil), eventsOverride...)
	}
	return res, nil
}

// parseResponse extracts the strict-JSON response from raw model
// output. Tolerates a leading/trailing markdown code fence — some
// providers wrap JSON output despite explicit "no fences" instructions.
func parseResponse(content string) (*Result, error) {
	s := strings.TrimSpace(content)

	// Strip code fences if present.
	if strings.HasPrefix(s, "```") {
		// Drop first line (e.g. ```json) and trailing ```.
		nl := strings.Index(s, "\n")
		if nl > 0 {
			s = s[nl+1:]
		}
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
		s = strings.TrimSpace(s)
	}

	// If extra prose surrounds the JSON object, isolate the largest
	// {...} span. This is forgiving but bounded — we parse with
	// json.Unmarshal so structural mismatches still error out.
	if !strings.HasPrefix(s, "{") {
		i := strings.Index(s, "{")
		if i < 0 {
			return nil, fmt.Errorf("no JSON object in response: %q", content)
		}
		s = s[i:]
	}
	if !strings.HasSuffix(s, "}") {
		i := strings.LastIndex(s, "}")
		if i < 0 {
			return nil, fmt.Errorf("no JSON close in response: %q", content)
		}
		s = s[:i+1]
	}

	var raw struct {
		Name      string   `json:"name"`
		Events    []string `json:"events"`
		Script    string   `json:"script"`
		Rationale string   `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w (body=%q)", err, s)
	}

	if raw.Script == "" {
		return nil, fmt.Errorf("response missing script")
	}

	return &Result{
		Name:      raw.Name,
		Script:    raw.Script,
		Events:    raw.Events,
		Rationale: raw.Rationale,
	}, nil
}
