// Package errors provides a structured error taxonomy for nerv.
//
// Each error carries a Category, operation name, optional path,
// wrapped cause, and a user-facing Hint. The fail-open policy
// (derived from spec/failure-modes.md) determines whether a given
// error category should allow the request to proceed or block it.
package errors

import (
	"errors"
	"fmt"
)

// Category classifies an error into one of eight domains.
type Category string

const (
	ErrConfig   Category = "config"
	ErrEnvelope Category = "envelope"
	ErrHandler  Category = "handler"
	ErrPolicy   Category = "policy"
	ErrAdapter  Category = "adapter"
	ErrInstall  Category = "install"
	ErrSpec     Category = "spec"
	ErrInternal Category = "internal"
)

// failOpenCategories lists categories that default to fail-open.
// ErrEnvelope is included because parse errors (the common case)
// fail-open; size-limit enforcement is a separate policy check.
var failOpenCategories = map[Category]bool{
	ErrConfig:   true,
	ErrEnvelope: true,
	ErrHandler:  true,
	ErrAdapter:  true,
	ErrInternal: true,
}

// Error is the structured error type for the nerv runtime.
type Error struct {
	Category Category // domain of the failure
	Op       string   // operation that failed
	Path     string   // file or resource path, if relevant
	Err      error    // underlying cause
	Hint     string   // user-facing suggestion
}

// Error formats as "op: category: err".
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %s", e.Op, e.Category, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Category)
}

// Unwrap returns the underlying error for use with errors.Is/As.
func (e *Error) Unwrap() error {
	return e.Err
}

// Wrap creates a new [Error] with the given fields.
func Wrap(cat Category, op, path string, err error, hint string) *Error {
	return &Error{
		Category: cat,
		Op:       op,
		Path:     path,
		Err:      err,
		Hint:     hint,
	}
}

// IsFailOpen reports whether err belongs to a fail-open category.
// Returns false for nil errors or non-nerv errors.
func IsFailOpen(err error) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return failOpenCategories[e.Category]
}

// CategoryOf returns the category string if err is a nerv error,
// or "unknown" for non-nerv errors.
func CategoryOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return string(e.Category)
	}
	return "unknown"
}

// Is reports whether err (or any error in its chain) matches cat.
func Is(err error, cat Category) bool {
	var e *Error
	if !errors.As(err, &e) {
		return false
	}
	return e.Category == cat
}
