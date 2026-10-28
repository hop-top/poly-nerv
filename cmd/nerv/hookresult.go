package main

import (
	"os"

	"github.com/spf13/viper"
	"hop.top/kit/go/console/output"
)

// hookResult is the structured outcome of a hook mutation (`add`,
// `remove`). Rendered through the same output.Render pathway as
// status/doctor so --format json|yaml|table all hold.
type hookResult struct {
	Hook   string `table:"Hook"   json:"hook"             yaml:"hook"`
	Action string `table:"Action" json:"action"           yaml:"action"`
	Source string `table:"Source" json:"source,omitempty" yaml:"source,omitempty"`
	Purged bool   `table:"Purged" json:"purged,omitempty" yaml:"purged,omitempty"`
}

// renderResult writes v to stdout in the active --format, defaulting
// to table for humans. Mirrors the pathway status, doctor, lint, and
// install-status already use.
func renderResult(v any) error {
	format := viper.GetString("format")
	if format == "" {
		format = output.Table
	}
	return output.Render(os.Stdout, format, v)
}
