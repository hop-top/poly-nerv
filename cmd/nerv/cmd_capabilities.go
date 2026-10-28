package main

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"hop.top/kit/go/ai/toolspec"
	speccli "hop.top/kit/go/ai/toolspec/cli"
	"hop.top/kit/go/console/cli"
	"hop.top/kit/go/console/output"
)

// capabilitiesSchemaVersion is the CLI-surface schema version the
// capabilities document claims. Distinct from the binary semver:
// it evolves when the introspection shape changes.
const capabilitiesSchemaVersion = "1.0"

// capabilityLeaf is one invocable leaf in the capabilities document:
// full command path, leaf name, contract annotations, and the leaf's
// local flag surface.
type capabilityLeaf struct {
	Path       string                  `table:"Path"        json:"path"                   yaml:"path"`
	Name       string                  `table:"Name"        json:"name"                   yaml:"name"`
	Short      string                  `table:"Description" json:"short,omitempty"        yaml:"short,omitempty"`
	SideEffect string                  `table:"Side Effect" json:"side_effect,omitempty"  yaml:"side_effect,omitempty"`
	Idempotent string                  `table:"Idempotent"  json:"idempotent,omitempty"   yaml:"idempotent,omitempty"`
	DryRun     bool                    `table:"-"           json:"dry_run_supported"      yaml:"dry_run_supported"`
	Flags      []toolspec.ManifestFlag `table:"-"           json:"flags,omitempty"        yaml:"flags,omitempty"`
}

// capabilitiesDoc is the machine-readable capability surface the
// binary serves: agents parse leaves[].path / leaves[].name to
// discover what they may invoke without scraping help text.
type capabilitiesDoc struct {
	Tool          string           `json:"tool"           yaml:"tool"`
	Version       string           `json:"version"        yaml:"version"`
	SchemaVersion string           `json:"schema_version" yaml:"schema_version"`
	Leaves        []capabilityLeaf `json:"leaves"         yaml:"leaves"`
}

// capabilitiesCmd mounts the capability introspection leaf. The
// projection reuses kit's toolspec walker (the same machinery behind
// `<tool> spec`), so annotations, deprecation filtering, and flag
// discovery stay kit-canonical; only the emitted shape differs —
// a flat leaves list keyed by string path.
func capabilitiesCmd(root *cli.Root) *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities",
		Short: "Emit the machine-readable command surface",
		Long: `Describe every invocable leaf of the nerv CLI as structured data:
full command path, leaf name, side-effect and idempotency contract,
dry-run support, and per-leaf flags. Agents consume this instead of
scraping help text; humans get a table.

Read-only; honors --format (table, json, yaml). The document mirrors
the live command tree, so a leaf listed here is invocable as printed.`,
		Args: cobra.NoArgs,
		Annotations: map[string]string{
			"kit/side-effect":    "read",
			"kit/idempotent":     "yes",
			"kit/top-level-verb": "true",
			// Skip the deprecation-warning middleware so the emitted
			// document stream stays clean (same marker kit's own spec
			// command carries). The leaf still lists itself in the
			// projection — it is part of the invocable surface.
			"kit/spec-command": "true",
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			m := speccli.EmitManifest(root, capabilitiesSchemaVersion)

			doc := capabilitiesDoc{
				Tool:          m.Tool,
				Version:       m.Version,
				SchemaVersion: m.SchemaVersion,
				Leaves:        make([]capabilityLeaf, 0, len(m.Commands)),
			}
			for _, mc := range m.Commands {
				if len(mc.Path) == 0 {
					continue
				}
				doc.Leaves = append(doc.Leaves, capabilityLeaf{
					Path:       strings.Join(mc.Path, " "),
					Name:       mc.Path[len(mc.Path)-1],
					Short:      mc.Short,
					SideEffect: mc.SideEffect,
					Idempotent: mc.Idempotent,
					DryRun:     mc.DryRunSupported,
					Flags:      mc.Flags,
				})
			}

			format := viper.GetString("format")
			if format == "" {
				format = output.Table
			}
			if format == output.Table {
				// Humans get the leaf table; the envelope fields
				// (tool/version/schema) are noise in table mode.
				return output.Render(os.Stdout, format, doc.Leaves)
			}
			return output.Render(os.Stdout, format, doc)
		},
	}
}
