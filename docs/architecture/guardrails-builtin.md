# ADR: guardrails built-in — block/rewrite dangerous tool calls

Status: proposed
Date: 2026-04-28
Author: jadb

## Context

AI agent harnesses (Claude Code, Codex, Gemini, OpenCode, etc.) drive
shell commands on behalf of the user. Agents make mistakes. A
mistaken `git push --force` to `main`, `rm -rf $HOME`, or
`curl ... | sudo bash` is unrecoverable in the worst case and costly
to recover from in the best case.

[`jaeyeom/claude-toolbox`](https://github.com/jaeyeom/claude-toolbox)
ships a `git-guardrails.sh` PreToolUse hook that pattern-matches a
small set of Claude-Code-specific footguns and blocks them. We want
the same protection, but:

1. CLI-agnostic — work for every adapter nerv supports
2. Extensible — declarative rule packs, not hard-coded regexes
3. Mode-aware — block, rewrite, ask, escalate, allow
4. Reversible — destructive ops backed up before exec
5. Default-safe — out-of-the-box protection without user action

This ADR defines the in-process built-in `guardrails` that ships with
nervd.

## Decision

Add `internal/hookpkg/builtins/guardrails/` as the third built-in
(after `openquestions` and `aps-lookup`). It subscribes to the
canonical `PreToolUse` event and emits a decision after evaluating
the envelope's tool + args against a layered set of rule packs.

Rule packs are declarative YAML files in `spec/rulepacks/<name>.yaml`
loaded at daemon startup. v1 ships with: `git.yaml`, `rm.yaml`,
`gh.yaml`, `curl.yaml`, `wget.yaml`, `sudo.yaml`.

## Goals

- Default-on protection for the v1 rule set; user opts out per-rule
- Single config knob path: `hooks.yaml → builtin.guardrails.*`
- Predictable: every rule has explicit mode + reason
- Extensible: new rules via YAML, no recompile
- Reversible: backup-before-exec for destructive paths
- Auditable: every decision emits a bus event

## Non-goals

- Generic linter — ShellCheck does that better
- Network policy — that's the firewall layer's job
- Per-file ACL — out of scope; OS handles it
- Replacing the OS `sudo` policy — we add a layer, not replace one
- Cross-platform Windows support in v1 — POSIX shell only

## Architecture

### Event flow

The built-in fires on every `PreToolUse` event. The flow is a
short-circuit pipeline; the first rule that matches decides:

[guardrails-flow-v1.mmd](guardrails-builtin/guardrails-flow-v1.mmd)

### Decision interface (extensible — Q2 from kickoff)

Three layers; each implements the next when escalation is needed.
Lower layers can be configured per-rule, higher layers are the
fallback path.

```pseudocode
interface ToolUseDecision {                  // synchronous, no I/O
    Evaluate(envelope) → Outcome             // allow | deny | rewrite
}

interface InteractiveDecision extends ToolUseDecision {
    AskUser(prompt) → Approval               // sync stdin/stderr prompt
}

interface Decision extends InteractiveDecision {
    Escalate(target, payload) → DecisionRef  // async via kit/bus inbox
    Resolve(ref) → Outcome                   // poll/wait final state
}
```

A rule's `mode` selects which interface depth is used:

| Mode       | Interface           | What it does |
|---|---|---|
| `allow`    | ToolUseDecision     | Pass-through; rule disabled |
| `block`    | ToolUseDecision     | Deny + reason |
| `rewrite`  | ToolUseDecision     | Mutate `tool_input.command`; emit `rewrite` outcome |
| `hitl`     | InteractiveDecision | Ask user via stderr; block daemon until reply |
| `manager`  | Decision            | Publish `inbox_message_added` → manager profile; defer |
| `peer-N`   | Decision            | Same, route to N peers; resolve when N approve |

Manager + peer-N reuse the inbox-message events shipped with the kit/bus adoption.

### Rule pack format

```pseudocode
# spec/rulepacks/git.yaml
name: git
version: 1
description: Block destructive git operations.
applies_to:
  tool: Bash         # only matches Bash tool calls
rules:
  - id: git-no-verify
    description: --no-verify skips hooks
    severity: high
    pattern: '\bgit\b[^|;]*\bcommit\b[^|;]*--no-verify'
    default_mode: block
    rewrites:
      - strategy: strip_flag
        flag: --no-verify
    reason: |
      The --no-verify flag bypasses commit hooks. Investigate the
      hook failure instead of skipping it.

  - id: git-push-force
    description: force-push without lease
    severity: high
    pattern: '\bgit\b\s+push\b[^|;]*--force(\s|$)'
    default_mode: rewrite     # safer by default — rewrite to lease
    rewrites:
      - strategy: replace_token
        from: --force
        to: --force-with-lease
    destructive: true         # → triggers backup before exec
```

### Backup contract

Any rule with `destructive: true` MUST backup state before letting the
command proceed. Backup happens **after** the user has confirmed
(post-HITL) but **before** the actual tool execution.

Storage: `StateDir("nerv")/backups/<project-key>/<ts>-<ruleid>.tar.gz`

`<project-key>` = bare-root absolute path of the repo (option a from
kickoff). For non-git directories, `cwd` is the key. Worktrees of the
same bare repo share a key.

What gets backed up depends on the rule. Each rule declares a
`backup_strategy`:

| Strategy | What it captures | Used by |
|---|---|---|
| `path_archive`     | tar+gz of the target paths | `rm -rf <path>` |
| `git_stash_full`   | `git stash push -u`, plus tar of `.git/refs` and working tree | `git reset --hard` |
| `git_untracked`    | tar of untracked files (`git ls-files --others --exclude-standard`) | `git clean -fd*` |
| `remote_bundle`    | `git bundle create` of remote ref before push | `git push --force*` |
| `script_capture`   | save the piped script content + URL | `curl \| sh`, `wget \| sh` |
| `none`             | skipped | non-destructive rewrites |

If backup fails (disk full, permission denied, etc.), the rule
upgrades its decision to `block` regardless of mode. Better to
inconvenience the agent than lose state silently.

### Backup retention + GC

| Knob | Default | Configurable |
|---|---|---|
| TTL (per backup) | 2 days | yes — `builtin.guardrails.backup.ttl_hours` |
| Per-project count cap | 50 | yes — `builtin.guardrails.backup.max_per_project` |
| Total size cap | 1 GB | yes — `builtin.guardrails.backup.max_total_bytes` |
| GC trigger | daemon upgrade (version bump) | no |

GC runs once per nervd boot when the on-disk version stamp differs from
the binary's `version` constant. Rationale: predictable, no
background-timer complexity, and the upgrade boundary is when state
schema may have changed anyway.

GC algorithm:

```pseudocode
for each project_key under StateDir("nerv")/backups/:
    archives = list(project_key)
    archives.sort(by_mtime, descending)
    # 1. age-based: delete anything older than TTL
    for a in archives:
        if now - a.mtime > ttl: delete(a)
    # 2. count-based: keep newest N per project
    for a in archives[max_per_project:]:
        delete(a)
# 3. size-based (across all projects):
total = sum(size for a in all_archives)
while total > max_total_bytes:
    delete(oldest globally)
    total -= deleted.size
```

### Project-key derivation

```pseudocode
func projectKey(cwd string) string {
    # walk up from cwd looking for .git
    dir := cwd
    for {
        if exists(dir + "/.git"): return absolute(dir)
        if dir == "/": return absolute(cwd)   # not a git repo
        dir = parent(dir)
    }
}
```

Bare-root repos (`git hop`-managed): `.git` in `hops/<branch>/.git`
is a gitfile that points to the bare root. Walking up still finds
the worktree's `.git` first; we resolve the gitfile to get the bare
root, which IS the project key. Worktrees of the same bare repo
collapse to the same key.

### Configuration surface

In `hooks.yaml` (user layer):

```pseudocode
builtin:
  guardrails:
    enabled: true                    # opt-out toggle (default: true)
    backup:
      ttl_hours: 48                  # default 48 (= 2 days)
      max_per_project: 50
      max_total_bytes: 1073741824    # 1 GB
    rules:                           # per-rule override map
      git-push-force:
        mode: hitl                   # change default rewrite → ask
      git-reset-hard:
        mode: peer-2                 # require 2 peer approvals
      rm-rf-home:
        mode: block                  # explicit block (already default)
```

Rule packs are loaded from `spec/rulepacks/*.yaml` at startup. User
config layers on top: per-rule `mode` overrides the pack's
`default_mode`.

## v1 rule surface

15 rules across 6 packs. Default modes chosen for "safer than sorry".

### `git.yaml` (8 rules)

| id | match | default | rewrite |
|---|---|---|---|
| git-no-verify | `git commit --no-verify` | block | strip `--no-verify` |
| git-add-all | `git add . / -A / --all` | block | — (must list files) |
| git-push-force | `git push --force` (no lease) | rewrite | `--force-with-lease` |
| git-push-force-protected | `git push --force*` to main/master/trunk/release | block | — |
| git-reset-hard | `git reset --hard` | hitl | — |
| git-clean-fd | `git clean -fd*` | hitl | — |
| git-filter-branch | `git filter-branch` / `git filter-repo` | block | — |
| git-config-githooks-disable | `git config githooks.{disable,skipUntrustedHooks}` | block | — |

### `rm.yaml` (4 rules)

| id | match | default | rewrite |
|---|---|---|---|
| rm-rf-root | `rm -rf /` | block | — |
| rm-rf-home | `rm -rf ~ \| $HOME \| /Users/<u>` | block | — |
| rm-rf-glob | `rm -rf *` (without explicit path) | hitl | — |
| rm-no-preserve | `rm -rf --no-preserve-root` | block | — |

### `gh.yaml` (1 rule)

| id | match | default | rewrite |
|---|---|---|---|
| gh-merge-admin | `gh pr merge --admin` | block | — |

### `curl.yaml` (1 rule)

| id | match | default | rewrite |
|---|---|---|---|
| curl-pipe-shell | `curl ... \| (sh\|bash\|sudo bash)` | block | — |

### `wget.yaml` (1 rule)

| id | match | default | rewrite |
|---|---|---|---|
| wget-pipe-shell | `wget ... -O- \| (sh\|bash)` | block | — |

### `sudo.yaml` (continues into bypass list — see `bypass.yaml`)

| id | match | default | rewrite |
|---|---|---|---|
| sudo-rm-rf | `sudo rm -rf <anything>` | hitl | — |
| sudo-curl-pipe | `sudo curl \| sh` | block | — |
| sudo-shutdown | `sudo (shutdown\|reboot\|halt\|poweroff)` | block | — |

That's 18 rules; pack count subject to refinement during build.

## Open questions

- **OQ1 — async resolution timeout.** When `mode: peer-2`, what
  happens if 2nd peer never responds? Default proposal: 24h timeout,
  fall back to `block`. Configurable via
  `builtin.guardrails.async_timeout_hours`.
- **OQ2 — backup of the agent's own scratch area.** nervd writes to
  `StateDir("nerv")/`. A guardrail-triggered backup MUST NOT capture
  the backups dir itself (recursive). Resolution: skip
  `StateDir("nerv")/backups/` in `path_archive` strategy.
- **OQ3 — rewrite confirmation.** `mode: rewrite` mutates the
  command silently. Should the agent be told? Proposal: emit the
  rewrite in the decision reason; agent harness can choose to
  display.

## Alternatives considered

- **External hook script** (like the source `git-guardrails.sh`).
  Rejected: needs per-CLI installation, no shared state, no backup
  contract, not extensible.
- **Block-only** (no rewrite/escalation). Rejected: too coarse;
  daily friction would push users to disable entirely, defeating the
  purpose.
- **Hard-coded rules in Go.** Rejected per Q3 in kickoff: rule packs
  belong with the other declarative specs; new rules should not
  require recompile.
- **Run rules in agent process, not daemon.** Rejected: every CLI
  adapter would need its own integration; the canonical-event
  pipeline already exists.

## Test plan

- Unit: each rule's regex against true-positive + false-positive
  corpus per rule pack (`internal/hookpkg/builtins/guardrails/testdata`)
- Integration: replay-mode validator with envelope fixtures matching
  every rule
- E2E: a "destructive smoke" test that:
  1. Configures `mode: block` for `rm-rf-home`
  2. Sends a fake envelope with `rm -rf $HOME`
  3. Asserts decision = deny + correct reason
- E2E backup: inject `mode: hitl`, simulate user approval, confirm
  archive lands in `StateDir("nerv")/backups/<key>/` with correct
  contents
- GC: simulate version bump, populate fake old archives, run boot,
  assert TTL/count/size invariants hold post-GC

## References

- Source script: [git-guardrails.sh](https://raw.githubusercontent.com/jaeyeom/claude-toolbox/refs/heads/main/plugins/git-guardrails/hooks/git-guardrails.sh)
- builtins-registry.md (registry pattern)
- kit-bus-adoption.md: bus event schema (inbox_message_* for async approval)
- spec/builtins.yaml schema (config layer)
- AGENTS.md: "Critical Thinking → NEVER destroy data" — this ADR
  operationalizes that rule.
