package redact

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hop.top/nerv/internal/envelope"
)

// fake joins parts at runtime. The sample credentials in these tests are
// split so no source line holds a complete credential-shaped literal:
// secret scanners (GitHub push protection and similar) would otherwise
// flag the intentional fake values. The joined string is byte-identical
// to the original sample.
func fake(parts ...string) string { return strings.Join(parts, "") }

// ---------- Bearer Token ----------

func TestRedact_BearerToken(t *testing.T) {
	e := New()
	positives := []string{
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9",
		fake("Bearer sk-", "abc123def456ghi789jkl012mno345pqr678"),
		"use Bearer AAAA.BBBB+CCCC/DDDD==",
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "Bearer ${REDACTED:token}") {
			t.Errorf("expected bearer redacted, got %q", out)
		}
	}

	negatives := []string{
		"Bearer ",      // no token after Bearer
		"bear token",   // not Bearer
		"Bearerless x", // no space
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:token}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- API Key ----------

func TestRedact_APIKey(t *testing.T) {
	e := New()
	positives := []string{
		"sk_abcdefghijklmnopqrstuvwx",
		"api-key1234567890abcdefghij",
		"token_abcdefghijklmnopqrstuv",
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:api_key}") {
			t.Errorf("expected api_key redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"application_context",   // too short suffix
		"skeleton",              // starts with sk but no _ or long alphanum
		"skill_testing_example", // "sk" prefix but not 20+ chars after
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:api_key}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- JWT ----------

func TestRedact_JWT(t *testing.T) {
	e := New()
	positives := []string{
		fake("eyJhbGciOiJIUzI1NiJ9.", "eyJzdWIiOiIxMjM0NTY3ODkwIn0.abc123def456"),
		fake("eyJhbGciOiJSUzI1NiJ9.", "eyJpc3MiOiJ0ZXN0In0.sig_value_here"),
		fake("token=eyJhbGciOiJFUzI1NiJ9.", "eyJleHAiOjk5OTk5OTk5OTl9.ecdsasig"),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:jwt}") {
			t.Errorf("expected jwt redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"com.example.service",
		"just.some.dots.here",
		"not.a.jwt.token.really",
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:jwt}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- Private Key ----------

func TestRedact_PrivateKey(t *testing.T) {
	e := New()
	positives := []string{
		fake("-----BEGIN ", "PRIVATE KEY-----\nMIIEvQ==\n-----END PRIVATE KEY-----"),
		fake("-----BEGIN ", "RSA PRIVATE KEY-----\ndata\n-----END RSA PRIVATE KEY-----"),
		fake("-----BEGIN ", "EC PRIVATE KEY-----\nkey\n-----END EC PRIVATE KEY-----"),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:private_key}") {
			t.Errorf("expected private_key redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"-----BEGIN PUBLIC KEY-----\ndata\n-----END PUBLIC KEY-----",
		"-----BEGIN CERTIFICATE-----\ndata\n-----END CERTIFICATE-----",
		"just a normal string with dashes ---",
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:private_key}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- AWS Access Key ----------

func TestRedact_AWSAccessKey(t *testing.T) {
	e := New()
	positives := []string{
		fake("AKIA", "IOSFODNN7EXAMPLE"),
		fake("aws_key=AKIA", "I44QH8DHBEXAMPLE"),
		fake("key: AKIA", "IOSFODNN7EXAMPL2"),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:aws_access_key}") {
			t.Errorf("expected aws_access_key redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"AKIA1234",             // too short (only 4 after AKIA)
		"AKIB1234567890123456", // wrong prefix
		"not an aws key at all",
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:aws_access_key}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- GitHub Token ----------

func TestRedact_GitHubToken(t *testing.T) {
	e := New()
	positives := []string{
		fake("ghp_", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn"),
		fake("ghs_", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn"),
		fake("github_pat_", "1234567890ABCDEFGHIJKL"),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:github_token}") {
			t.Errorf("expected github_token redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"ghp_short",           // too short
		"github_pat_tooshort", // too short
		"ghx_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmn", // wrong prefix
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:github_token}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- Connection String ----------

func TestRedact_ConnectionString_WithCreds(t *testing.T) {
	e := New()
	positives := []string{
		fake("postgresql://user:", "pass@localhost:5432/db"),
		fake("mongodb+srv://admin:", "secret@cluster0.example.net/mydb"),
		fake("redis://default:", "password@redis.example.com:6379"),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:connection_string}") {
			t.Errorf("expected connection_string redacted for %q, got %q", in, out)
		}
	}
}

func TestRedact_ConnectionString_NoCreds(t *testing.T) {
	e := New()
	negatives := []string{
		"postgresql://localhost",
		"redis://localhost:6379",
		"mongodb://localhost/test",
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:connection_string}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- Slack Token ----------

func TestRedact_SlackToken(t *testing.T) {
	e := New()
	positives := []string{
		fake("xoxb-", "123456789012-1234567890123-AbCdEfGhIjKl"),
		fake("xoxp-", "123456789012-1234567890123-1234567890123-abcdef1234"),
		fake("xoxe-", "1-ABCDEFGHIJKLMNOPQRSTUVWX"),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:slack_token}") {
			t.Errorf("expected slack_token redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"xoxb-short",                    // too short
		"xoxz-123456789012345678901234", // wrong prefix letter
		"normal text here",
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:slack_token}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- Timeout ----------

func TestRedact_Timeout(t *testing.T) {
	e := NewWithTimeout(1 * time.Nanosecond)
	// Generate large input to trigger timeout.
	big := strings.Repeat("Bearer token123 ", 100000)
	_, err := e.Redact(big)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected timeout in error, got %v", err)
	}
}

// ---------- HashContent ----------

func TestHashContent(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	result := HashContent(content)

	if !strings.Contains(result, `"sha256":`) {
		t.Errorf("missing sha256 field: %s", result)
	}
	if !strings.Contains(result, `"bytes":29`) {
		t.Errorf("missing or wrong bytes field: %s", result)
	}

	// Verify it parses as JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("HashContent output not valid JSON: %v", err)
	}
	if _, ok := parsed["sha256"]; !ok {
		t.Error("parsed JSON missing sha256 key")
	}
	if _, ok := parsed["bytes"]; !ok {
		t.Error("parsed JSON missing bytes key")
	}
}

// ---------- RedactEnvelope with full_payload ----------

func TestRedactEnvelope_FullPayload(t *testing.T) {
	e := New()
	env := &envelope.Envelope{
		Payload: json.RawMessage(`{"token":"Bearer sk_abcdefghijklmnopqrstuvwx"}`),
	}

	// With fullPayload=true, payload should remain unchanged.
	original := string(env.Payload)
	if err := e.RedactEnvelope(env, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(env.Payload) != original {
		t.Errorf("full_payload should skip redaction;\n  got:  %s\n  want: %s",
			string(env.Payload), original)
	}
}

func TestRedactEnvelope_Redacts(t *testing.T) {
	e := New()
	env := &envelope.Envelope{
		Payload: json.RawMessage(`{"auth":"Bearer sk_abcdefghijklmnopqrstuvwx"}`),
	}

	if err := e.RedactEnvelope(env, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := string(env.Payload)
	if strings.Contains(out, "sk_abcdefghijklmnopqrstuvwx") {
		t.Errorf("secret not redacted in envelope: %s", out)
	}
}

// ---------- AWS Secret Key ----------

func TestRedact_AWSSecretKey(t *testing.T) {
	e := New()
	positives := []string{
		fake("aws_secret_access_key = ", "wJalrXUtnFEMI/K7MDENG", "/bPxRfiCYEXAMPLEKEY"),
		fake("aws_secret_access_key=", "ABCDEFGHIJ1234567890", "abcdefghij1234567890"),
		fake("aws_secret_access_key =  ", "A1B2C3D4E5F6G7H8I9J0", "a1b2c3d4e5f6g7h8i9j0"),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "aws_secret_access_key=${REDACTED:aws_secret_key}") {
			t.Errorf("expected aws_secret_key redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"aws_secret_access_key = short",       // too short
		"aws_access_key_id = AKIAIOSFODNN7EX", // not secret key
		"some random config line",
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:aws_secret_key}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- Base64 Secret ----------

func TestRedact_Base64Secret(t *testing.T) {
	e := New()
	positives := []string{
		fake("secret: ", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnop"),
		fake("password=", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnop=="),
		fake("credential: ", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnop+/=="),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:base64_secret}") {
			t.Errorf("expected base64_secret redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"secret: short",
		"key: abc",
		"some_password_field in docs",
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:base64_secret}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}

// ---------- Dotenv Content ----------

func TestRedact_DotenvContent(t *testing.T) {
	e := New()
	positives := []string{
		fake("DATABASE_URL=postgres://user:", "pass@host/db"),
		fake("SECRET_KEY=", "mysupersecretvalue"),
		fake("API_TOKEN=", "abc123def456"),
	}
	for _, in := range positives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(out, "${REDACTED:env_line}") {
			t.Errorf("expected env_line redacted for %q, got %q", in, out)
		}
	}

	negatives := []string{
		"lowercase_key=value",     // lowercase start
		"= no key",                // no key
		"just some normal string", // no = pattern
	}
	for _, in := range negatives {
		out, err := e.Redact(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(out, "${REDACTED:env_line}") {
			t.Errorf("false positive for %q → %q", in, out)
		}
	}
}
