---
description: "Generate a hook script from intent — LLM authors handler + hooks.yaml entry"
argument-hint: "<description> [--lang sh|py] [--events <csv>] [--dry-run] [--name <slug>]"
allowed-tools: Bash(nerv:*)
---

# Author a hook from intent

Invoke the `nerv create` verb to generate a hook handler script and
`hooks.yaml` entry from a natural-language intent. The verb resolves an
LLM provider (anthropic / openai / ollama) via kit's `hop.top/kit/go/ai/llm`
abstraction, authors the script, and registers it through the same
`hookpkg.Install` path that `nerv add` uses.

## Argument parsing

- `<description>` (positional, REQUIRED): natural-language intent.
  Examples:
  - `"warn when files >1MB are written"`
  - `"block rm -rf in the project root"`
  - `"audit-log every shell command"`
- `--lang sh|py` (default `sh`): handler language.
- `--events <csv>` (optional): override inferred event subscription.
- `--name <slug>` (optional): override derived hook name.
- `--llm-provider <uri>` (optional): override default LLM provider
  (resolved from `$LLM_PROVIDER` or `~/.config/hop/llm.yaml`).
- `--dry-run`: print artifacts without writing.

## Execution

1. Validate args. Require `<description>`. If `--lang` is given,
   reject anything outside `{sh, py}`.

2. Run:

   ```sh
   nerv create "<description>" \
     [--lang <lang>] \
     [--events <csv>] \
     [--name <slug>] \
     [--llm-provider <uri>] \
     [--dry-run]
   ```

3. Surface stdout from `nerv create`. On non-zero exit, also surface
   stderr verbatim so the user sees the provider / lint / write
   failure message in full.

4. On success (exit 0, not `--dry-run`), suggest follow-ups:

   > Run `/nerv-test <name>` to exercise the new hook against a stub
   > envelope. Run `/nerv-doctor` to verify daemon + lockfile health.

## Notes

- Not idempotent: re-running with the same description re-invokes the
  LLM and may produce a different script. Use `nerv remove <name>`
  before re-creating for a clean slate.
- The verb handles all writes (`.nerv/hooks/<name>.<ext>`, hooks.yaml,
  lockfile). The slash command never touches the filesystem directly.

## Related skill

See `.agents/skills/nerv/SKILL.md` — sections "Authoring hooks" and
"Decisions" for the contract every generated handler must satisfy.

## Related commands

- `/nerv-test` — exercise the generated hook against synthetic envelopes
- `/nerv-doctor` — verify nerv is healthy before running the new hook
- `/nerv-disable` — disable the hook later without removing it
