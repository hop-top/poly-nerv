// Package create generates nerv hook handlers from natural-language
// intents using an LLM provider.
//
// The Generator interface decouples cmd/nerv from the underlying LLM
// transport — production code wires a kit/ai/llm-backed implementation
// via NewGenerator; tests inject a deterministic mock by constructing a
// Generator directly.
//
// Generation output (Result) is consumed by the nerv create verb, which
// writes the script to a temp file and registers it via hookpkg.Install.
package create

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

//go:embed prompts/handler.md
var handlerPromptTemplate string

// Result is the structured output of a single generation call.
type Result struct {
	// Name is a kebab-case slug for the generated hook.
	Name string
	// Lang is "sh" or "py" — mirrors the input language flag.
	Lang string
	// Script is the full handler source (with shebang).
	Script string
	// Events is the inferred (or override-honoured) event subscription.
	Events []string
	// Rationale is the LLM's one-line justification.
	Rationale string
}

// Generator authors a hook from a natural-language intent.
type Generator interface {
	// Generate runs the LLM-backed authoring loop and returns a Result.
	// eventsOverride, if non-empty, MUST appear verbatim in Result.Events.
	Generate(ctx context.Context, intent, lang string, eventsOverride []string) (*Result, error)
}

// ValidLangs enumerates the languages the generator (and the CLI verb)
// accept. Production code asserts membership before calling Generate.
var ValidLangs = []string{"sh", "py"}

// ValidateLang returns an error if lang is not in ValidLangs.
func ValidateLang(lang string) error {
	for _, l := range ValidLangs {
		if l == lang {
			return nil
		}
	}
	return fmt.Errorf("invalid lang %q (want one of %v)", lang, ValidLangs)
}

// FileExt returns the file extension to use for a given lang.
func FileExt(lang string) string {
	switch lang {
	case "py":
		return "py"
	default:
		return "sh"
	}
}

// nameSlugRE matches characters NOT allowed in a kebab-case slug.
var nameSlugRE = regexp.MustCompile(`[^a-z0-9-]+`)

// DeriveName derives a kebab-case slug from a free-form intent. Used as
// a fallback when the LLM returns no name AND the user did not supply
// --name. Keeps the first ~6 words, lowercases, replaces non-[a-z0-9]
// with `-`, collapses repeated `-`, trims leading/trailing `-`.
func DeriveName(intent string) string {
	words := strings.Fields(strings.ToLower(intent))
	if len(words) > 6 {
		words = words[:6]
	}
	slug := strings.Join(words, "-")
	slug = nameSlugRE.ReplaceAllString(slug, "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "hook"
	}
	return slug
}

// renderPrompt fills the embedded handler.md template with the call's
// inputs. Substitutions are simple string replacement — the template
// uses {{INTENT}}, {{LANG}}, {{EVENTS_OVERRIDE}} placeholders.
func renderPrompt(intent, lang string, eventsOverride []string) string {
	override := "(none — infer events from the intent)"
	if len(eventsOverride) > 0 {
		override = strings.Join(eventsOverride, ", ")
	}
	p := handlerPromptTemplate
	p = strings.ReplaceAll(p, "{{INTENT}}", intent)
	p = strings.ReplaceAll(p, "{{LANG}}", lang)
	p = strings.ReplaceAll(p, "{{EVENTS_OVERRIDE}}", override)
	return p
}

// ErrEmptyIntent is returned by Generators when intent is whitespace.
var ErrEmptyIntent = errors.New("create: empty intent")
