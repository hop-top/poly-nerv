---
description: "Install a hook into the current project — curated marketplace picker when no name given"
argument-hint: "[<hook-name>] [--source <url|path>]"
allowed-tools: Bash(nerv:*), Read
---

# Add a hook to the project

Invoke the `nerv` skill (Lifecycle → Install + adapters section) and
register a hook in `.nerv/hooks.yaml`. The hook source is either a
named entry in the curated marketplace, an explicit URL/local path
passed via `--source`, or — when no argument is given — picked from
the marketplace via a multi-select prompt.

## Argument parsing

- First positional: hook name (optional). When omitted, the slash
  command walks `marketplace/` and presents a curated multi-select.
- `--source <url|path>`: explicit source override. Accepts an HTTPS
  URL pointing at a handler artifact, or a local filesystem path.
- All other flags pass through to `nerv add` unchanged
  (`--refresh`, `--dry-run`, etc.).

## Execution

### Discovery branch — no positional argument

When `<hook-name>` is omitted **and** `--source` is not supplied, the
slash command performs marketplace discovery client-side (host AI
walks the tree + parses YAML; no `nerv` CLI verb required for this
phase):

1. **Walk** the marketplace tree from repo root:
   - `marketplace/examples/*/hook.yaml` — single-hook examples.
   - `marketplace/starters/*/hooks.yaml` — multi-hook starter packs
     (note plural `hooks.yaml`).

2. **Build** a record per entry:

   ```text
   {
     name:         <basename of containing dir>,
     category:     "example" | "starter",
     source-path:  marketplace/<category>s/<name>/   (local path)
     description:  hook.yaml `description:` if present,
                   else first non-blank line of sibling README.md,
                   else "(no description)"
   }
   ```

   For starter packs the `hooks.yaml` is a multi-hook bundle: pull
   the top-level pack `description` if present, otherwise summarise
   from the sibling `README.md` first line (e.g. "Ready-to-use
   nerv hooks for projects using AGENTS.md / CLAUDE.md patterns").

3. **Present** a multi-select picker grouped by category. Each row:

   ```text
   [examples]
     [ ] block-rm-rf       — Blocks destructive rm -rf / commands
     [ ] audit-log         — Logs all tool calls to a file for auditing
     [ ] slow-query-warn   — Warns when shell commands exceed 30 seconds

   [starters]
     [ ] agents-md         — Ready-to-use hooks for AGENTS.md / CLAUDE.md projects
   ```

   The user can pick zero or more entries across both groups.

4. **Install** each selected entry by invoking the underlying verb
   once per pick, passing the local marketplace path as `--source`:

   ```sh
   # for an example
   nerv add <name> --source marketplace/examples/<name>/

   # for a starter pack — install the bundle
   nerv add <name> --source marketplace/starters/<name>/
   ```

   Surface each command's stdout. If any install fails (non-zero
   exit), surface stderr verbatim and continue with the remaining
   picks; report failures in the final summary.

5. **Summarise**: print one line per pick — `OK <name>` or
   `FAIL <name> — <error>` — followed by:

   > Run `/nerv-doctor` to verify the new hooks are healthy.
   > Run `nerv daemon reload` if the daemon is running.

### Explicit branch — argument(s) supplied

When `<hook-name>` or `--source` is given, skip discovery and invoke
the verb directly with whichever args the user supplied:

```sh
nerv add <hook-name> [--source <url|path>] [--refresh] [--dry-run]
```

Surface stdout from `nerv add`. On non-zero exit, also surface stderr
so the user sees the lockfile / SHA-256 / network failure message
verbatim. After a successful add, suggest `nerv daemon reload` so the
new hook is picked up without restarting the daemon.

## Notes

- Discovery is **client-side**: the slash command (host AI) walks the
  tree and parses YAML on each invocation. No `nerv` CLI verb is
  needed for marketplace browsing — entries self-describe via their
  own `hook.yaml` / `hooks.yaml` + README.md, so there is no central
  index file to drift out of sync.
- Marketplace paths are repo-relative; resolve from the current
  working directory's repo root.
- If `marketplace/` is missing (e.g. running outside the nerv repo),
  print `marketplace/ not found in this repo; pass --source <url|path>
  or run from a checkout of hop.top/nerv` and exit non-zero.

## Related skill

See `.agents/skills/nerv/SKILL.md` (Lifecycle section + `nerv add`
notes) for the full author → install → test flow, the hooks.lock
contract, and marketplace integration plans.

## Related commands

- `/nerv-doctor` — verify daemon + lockfile health after install
- `/nerv-install` — register the daemon launchd/systemd unit
- `/nerv-test` — drive a single hook against a stub envelope
