package dispatch

import (
	"fmt"
	"testing"

	nerverrors "hop.top/nerv/internal/errors"
	"hop.top/nerv/internal/merger"
)

func TestHandleError_FailOpen(t *testing.T) {
	err := nerverrors.Wrap(nerverrors.ErrConfig, "load", "", fmt.Errorf("bad yaml"), "check syntax")
	result, remaining := HandleError(err)
	if remaining != nil {
		t.Fatalf("expected nil remaining, got %v", remaining)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Action != "allow" {
		t.Errorf("action = %q, want %q", result.Action, "allow")
	}
	if !result.Logged {
		t.Error("expected Logged = true")
	}
	if result.OrigErr != err {
		t.Error("OrigErr should preserve original error")
	}
}

func TestHandleError_FailOpen_Handler(t *testing.T) {
	err := nerverrors.Wrap(nerverrors.ErrHandler, "exec", "/hook.sh", fmt.Errorf("timeout"), "")
	result, remaining := HandleError(err)
	if remaining != nil {
		t.Fatalf("expected nil remaining, got %v", remaining)
	}
	if result == nil || result.Action != "allow" {
		t.Errorf("expected allow result, got %v", result)
	}
}

func TestHandleError_FailOpen_Internal(t *testing.T) {
	err := nerverrors.Wrap(nerverrors.ErrInternal, "dispatch", "", fmt.Errorf("nil ptr"), "")
	result, remaining := HandleError(err)
	if remaining != nil {
		t.Fatalf("expected nil remaining, got %v", remaining)
	}
	if result == nil || result.Action != "allow" {
		t.Errorf("expected allow result, got %v", result)
	}
}

func TestHandleError_NotFailOpen(t *testing.T) {
	err := nerverrors.Wrap(nerverrors.ErrPolicy, "check", "", fmt.Errorf("denied"), "")
	result, remaining := HandleError(err)
	if result != nil {
		t.Fatalf("expected nil result for non-fail-open, got %v", result)
	}
	if remaining != err {
		t.Errorf("expected original error returned, got %v", remaining)
	}
}

func TestHandleError_NotFailOpen_Install(t *testing.T) {
	err := nerverrors.Wrap(nerverrors.ErrInstall, "install", "/bin/hook", fmt.Errorf("perm"), "")
	result, remaining := HandleError(err)
	if result != nil {
		t.Fatalf("expected nil result for ErrInstall, got %v", result)
	}
	if remaining != err {
		t.Errorf("expected original error returned, got %v", remaining)
	}
}

func TestHandleError_Nil(t *testing.T) {
	result, remaining := HandleError(nil)
	if result != nil {
		t.Errorf("expected nil result for nil error, got %v", result)
	}
	if remaining != nil {
		t.Errorf("expected nil remaining for nil error, got %v", remaining)
	}
}

func TestHandleError_NonNerv(t *testing.T) {
	err := fmt.Errorf("some random error")
	result, remaining := HandleError(err)
	if remaining != nil {
		t.Fatalf("non-nerv errors should fail-open, got remaining: %v", remaining)
	}
	if result == nil {
		t.Fatal("expected non-nil result for non-nerv error")
	}
	if result.Action != "allow" {
		t.Errorf("action = %q, want %q", result.Action, "allow")
	}
	if result.OrigErr != err {
		t.Error("OrigErr should preserve original error")
	}
}

func TestMustAllow_FailOpen(t *testing.T) {
	err := nerverrors.Wrap(nerverrors.ErrConfig, "load", "", fmt.Errorf("bad"), "")
	dec, ok := MustAllow(err)
	if !ok {
		t.Fatal("expected ok=true for fail-open error")
	}
	if dec.Action != merger.Allow {
		t.Errorf("action = %q, want %q", dec.Action, merger.Allow)
	}
	if dec.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestMustAllow_NotFailOpen(t *testing.T) {
	err := nerverrors.Wrap(nerverrors.ErrPolicy, "check", "", fmt.Errorf("denied"), "")
	dec, ok := MustAllow(err)
	if ok {
		t.Fatal("expected ok=false for non-fail-open error")
	}
	if dec.Action != "" {
		t.Errorf("expected zero decision, got action=%q", dec.Action)
	}
}
