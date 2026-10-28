package create

import (
	"strings"
	"testing"

	"hop.top/kit/go/ai/llm"
)

// TestSanitizeURIForLog locks in the credential-stripping rules
// applied to provider URIs before they land in user-facing errors
// or logs. The contract: ?query, #fragment, and user:pass@ are
// dropped; scheme + host + path remain so the operator can still
// recognise which provider failed.
func TestSanitizeURIForLog(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty renders default marker",
			in:   "",
			want: "(default)",
		},
		{
			name: "query params stripped",
			in:   "anthropic://claude-sonnet-4-5?api_key=secret123",
			want: "anthropic://claude-sonnet-4-5",
		},
		{
			name: "multiple query params stripped",
			in:   "openai://gpt-4o?api_key=sk-abc&org=acme",
			want: "openai://gpt-4o",
		},
		{
			name: "fragment stripped",
			in:   "ollama://localhost:11434/llama3#token=xyz",
			want: "ollama://localhost:11434/llama3",
		},
		{
			name: "userinfo stripped",
			in:   "anthropic://user:pass@claude-sonnet-4-5",
			want: "anthropic://claude-sonnet-4-5",
		},
		{
			name: "combined userinfo + query + fragment stripped",
			in:   "openai://alice:s3cret@gpt-4o?api_key=leak#tag=v",
			want: "openai://gpt-4o",
		},
		{
			name: "no creds passes through",
			in:   "anthropic://claude-sonnet-4-5",
			want: "anthropic://claude-sonnet-4-5",
		},
		{
			name: "host:port preserved",
			in:   "ollama://localhost:11434/llama3",
			want: "ollama://localhost:11434/llama3",
		},
		{
			name: "scheme-less garbage drops query tail anyway",
			in:   "garbage?api_key=secret",
			want: "garbage",
		},
		{
			name: "at-sign without authority still drops tail",
			in:   "bad@form?api_key=leak",
			want: "bad@form",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeURIForLog(c.in)
			if got != c.want {
				t.Errorf("sanitizeURIForLog(%q) = %q, want %q",
					c.in, got, c.want)
			}
			// Defensive: regardless of input, output must never carry
			// '?' or '#' (we drop both unconditionally).
			if strings.ContainsAny(got, "?#") {
				t.Errorf("output %q still carries ? or #", got)
			}
		})
	}
}

// TestNewGenerator_ErrorSanitisesURI exercises the error path so a
// caller passing ?api_key=... never sees the secret echoed back. The
// generator should fail because the bogus scheme has no registered
// adapter; the resulting message must contain the redacted URI and
// MUST NOT contain the secret tail.
func TestNewGenerator_ErrorSanitisesURI(t *testing.T) {
	const secret = "verysecret-do-not-leak"
	uri := "noprovider-bogus-scheme-zzz://m?api_key=" + secret

	_, err := NewGenerator(uri)
	if err == nil {
		t.Fatal("NewGenerator: expected error for bogus scheme")
	}
	msg := err.Error()
	if strings.Contains(msg, secret) {
		t.Errorf("error leaked api_key: %q", msg)
	}
	if strings.Contains(msg, "api_key=") {
		t.Errorf("error retained query: %q", msg)
	}
	// Sanity: the redacted host part should still be present so the
	// operator knows which provider was attempted.
	if !strings.Contains(msg, "noprovider-bogus-scheme-zzz://m") {
		t.Errorf("error lost provider context: %q", msg)
	}
}

// Compile-time guard: sanitizeURIForLog must accept the same string
// form renderURI emits. If renderURI ever returns a multi-line or
// otherwise structured payload, this guard catches it.
func TestSanitizeURIForLog_AcceptsRenderURIOutput(t *testing.T) {
	u := llm.URI{
		Scheme: "openai",
		Host:   "",
		Model:  "gpt-4o",
		Params: map[string]string{"api_key": "leak"},
	}
	rendered := renderURI(u)
	if !strings.Contains(rendered, "leak") {
		t.Fatalf("renderURI dropped params unexpectedly: %q", rendered)
	}
	if got := sanitizeURIForLog(rendered); strings.Contains(got, "leak") {
		t.Errorf("sanitize did not strip query: %q -> %q", rendered, got)
	}
}
