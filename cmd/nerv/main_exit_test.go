package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"hop.top/kit/go/console/output"
)

type envErr struct{ env *output.Error }

func (e envErr) Error() string             { return "wrapped" }
func (e envErr) AsCLIError() *output.Error { return e.env }

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"envelope code wins", envErr{&output.Error{ExitCode: 4}}, 4},
		{"envelope wins over usage text", envErr{&output.Error{ExitCode: 3}}, 3},
		{"envelope zero falls through to generic", envErr{&output.Error{}}, 1},
		{"nil envelope falls through", envErr{nil}, 1},
		{"wrapped envelope", fmt.Errorf("ctx: %w", envErr{&output.Error{ExitCode: 5}}), 5},
		{"canceled", context.Canceled, 130},
		{"wrapped canceled", fmt.Errorf("x: %w", context.Canceled), 130},
		{"usage", errors.New("unknown flag: --nope"), 2},
		{"generic", errors.New("boom"), 1},
	}
	for _, tc := range cases {
		if got := exitCodeFor(tc.err); got != tc.want {
			t.Errorf("%s: exitCodeFor = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestIsUsageError(t *testing.T) {
	yes := []string{
		"unknown flag: --x", "unknown shorthand flag: 'x' in -x",
		`unknown command "foo" for "nerv"`, "flag needs an argument: --format",
		"invalid argument \"x\" for \"--last\"", "accepts 1 arg(s), received 0",
		"requires at least 1 arg(s), only received 0",
	}
	for _, s := range yes {
		if !isUsageError(errors.New(s)) {
			t.Errorf("%q should be a usage error", s)
		}
	}
	for _, s := range []string{"boom", "open foo: no such file", "an unknown flag: x"} {
		if isUsageError(errors.New(s)) {
			t.Errorf("%q should not be a usage error", s)
		}
	}
}
