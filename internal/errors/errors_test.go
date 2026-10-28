package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestWrap(t *testing.T) {
	inner := fmt.Errorf("boom")
	e := Wrap(ErrConfig, "loadConfig", "/etc/nerv.yaml", inner, "check syntax")

	if e.Category != ErrConfig {
		t.Errorf("Category = %q, want %q", e.Category, ErrConfig)
	}
	if e.Op != "loadConfig" {
		t.Errorf("Op = %q, want %q", e.Op, "loadConfig")
	}
	if e.Path != "/etc/nerv.yaml" {
		t.Errorf("Path = %q, want %q", e.Path, "/etc/nerv.yaml")
	}
	if e.Err != inner {
		t.Errorf("Err = %v, want %v", e.Err, inner)
	}
	if e.Hint != "check syntax" {
		t.Errorf("Hint = %q, want %q", e.Hint, "check syntax")
	}
}

func TestError_Format(t *testing.T) {
	inner := fmt.Errorf("bad json")
	e := Wrap(ErrHandler, "runHook", "", inner, "")

	want := "runHook: handler: bad json"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestError_FormatNilErr(t *testing.T) {
	e := Wrap(ErrPolicy, "enforce", "", nil, "")

	want := "enforce: policy"
	if got := e.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestUnwrap(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	e := Wrap(ErrAdapter, "detect", "", inner, "")

	if unwrapped := e.Unwrap(); unwrapped != inner {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, inner)
	}

	// Verify errors.Is works through the chain.
	if !errors.Is(e, inner) {
		t.Error("errors.Is failed to find inner error")
	}
}

func TestIsFailOpen(t *testing.T) {
	tests := []struct {
		cat  Category
		want bool
	}{
		{ErrConfig, true},
		{ErrEnvelope, true},
		{ErrHandler, true},
		{ErrPolicy, false},
		{ErrAdapter, true},
		{ErrInstall, false},
		{ErrSpec, false},
		{ErrInternal, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.cat), func(t *testing.T) {
			err := Wrap(tt.cat, "op", "", fmt.Errorf("x"), "")
			if got := IsFailOpen(err); got != tt.want {
				t.Errorf("IsFailOpen(%s) = %v, want %v", tt.cat, got, tt.want)
			}
		})
	}
}

func TestIsFailOpen_Nil(t *testing.T) {
	if IsFailOpen(nil) {
		t.Error("IsFailOpen(nil) = true, want false")
	}
}

func TestIsFailOpen_NonNerv(t *testing.T) {
	if IsFailOpen(fmt.Errorf("plain error")) {
		t.Error("IsFailOpen(plain) = true, want false")
	}
}

func TestIs(t *testing.T) {
	err := Wrap(ErrSpec, "validate", "hook.yaml", fmt.Errorf("missing field"), "")

	if !Is(err, ErrSpec) {
		t.Error("Is(ErrSpec) = false, want true")
	}
	if Is(err, ErrConfig) {
		t.Error("Is(ErrConfig) = true, want false")
	}
}

func TestIs_Wrapped(t *testing.T) {
	inner := Wrap(ErrInstall, "install", "", fmt.Errorf("perm denied"), "")
	outer := fmt.Errorf("setup failed: %w", inner)

	if !Is(outer, ErrInstall) {
		t.Error("Is through fmt.Errorf wrapping failed")
	}
}

func TestIs_Nil(t *testing.T) {
	if Is(nil, ErrConfig) {
		t.Error("Is(nil, ErrConfig) = true, want false")
	}
}
