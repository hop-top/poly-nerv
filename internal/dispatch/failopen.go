package dispatch

import (
	stderrors "errors"
	"fmt"
	"log"

	nerverrors "hop.top/nerv/internal/errors"
	"hop.top/nerv/internal/merger"
)

// FailOpenResult represents the outcome of fail-open handling.
type FailOpenResult struct {
	Action  string // always "allow" for fail-open
	Logged  bool   // whether the error was logged
	OrigErr error  // original error preserved for observability
}

// HandleError is the single place that consumes spec/failure-modes.md.
// Every error that maps to fail-open is converted here into:
// "log warning, return allow, continue".
//
// Non-fail-open errors are returned as-is for the caller to handle.
// Non-nerv errors (unknown origin) default to fail-open since nerv
// should never block the host tool.
func HandleError(err error) (*FailOpenResult, error) {
	if err == nil {
		return nil, nil
	}

	// Non-nerv errors default to fail-open: nerv must never block.
	if !nerverrors.IsFailOpen(err) {
		var uerr *nerverrors.Error
		if stderrors.As(err, &uerr) {
			// nerv error that is NOT fail-open; caller must handle.
			return nil, err
		}

		// Non-nerv error → fail-open by default.
		log.Printf("dispatch: fail-open (non-nerv error): %v", err)
		return &FailOpenResult{
			Action:  "allow",
			Logged:  true,
			OrigErr: err,
		}, nil
	}

	log.Printf("dispatch: fail-open (%s): %v", nerverrors.CategoryOf(err), err)
	return &FailOpenResult{
		Action:  "allow",
		Logged:  true,
		OrigErr: err,
	}, nil
}

// MustAllow wraps HandleError for the common pattern: on fail-open,
// return an allow decision; otherwise signal the caller to propagate.
func MustAllow(err error) (merger.Decision, bool) {
	result, remaining := HandleError(err)
	if remaining != nil {
		return merger.Decision{}, false
	}
	if result == nil {
		// nil error
		return merger.Decision{}, false
	}
	return merger.Decision{
		Action:  merger.Allow,
		Message: fmt.Sprintf("fail-open: %v", result.OrigErr),
	}, true
}
