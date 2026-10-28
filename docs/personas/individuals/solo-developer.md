# Persona: Solo Developer (nerv Extension)

**Primary Role:** Independent engineer running multiple AI CLIs
(claude, codex, copilot, gemini, opencode) on personal machine.
Wants ONE place to write hooks that work everywhere, and ONE
place to surface session-level events (open questions, drift,
tool usage) without per-CLI plumbing.

---

## Base Goals

- Ship features fast across many concurrent projects without
  per-tool friction.
- Keep AI workflows safe (block destructive commands, audit
  agent decisions) without yak-shaving security infrastructure.
- Recover prior context (decisions, dead-ends, fixes) without
  re-doing investigation.

## Base Pain Points

- Every AI CLI has a different hook format. Writing the same
  `block rm -rf /` rule for claude, codex, copilot is 3 copies.
- Hook configuration scattered across `~/.claude/`, `~/.codex/`,
  `~/.copilot/` — no single source of truth.
- No way to react to "agent emitted open questions and pivoted"
  patterns without manually noticing.
- Onboarding a new built-in is opaque: which built-ins exist?
  what does each do? what events does each fire on?

---

## nerv Interaction Patterns

### One-time setup

```pseudocode
$ nerv init
// detects installed CLIs, prompts through built-in enable/disable,
// writes ~/.config/nerv/hooks.yaml (user layer)
```

### Day-to-day: passive

nerv daemon runs in the background. Hooks fire on every AI CLI
event. Decisions are surfaced inline in the CLI; inbox writes
appear in `$XDG_STATE_HOME/nerv/inbox/<agent_id>/`.

### Triage: open-questions inbox

```pseudocode
$ ls $XDG_STATE_HOME/nerv/inbox/jad/open-questions/
2026-04-26-15:42-<short-id>.md
2026-04-26-16:07-<short-id>.md

$ $EDITOR $XDG_STATE_HOME/nerv/inbox/jad/open-questions/2026-04-26-15:42-*.md
// answer inline OR check "Promote to tlc task" boxes

$ nerv inbox promote $XDG_STATE_HOME/nerv/inbox/jad/open-questions/2026-04-26-15:42-*.md
// emits inbox_message_promoted; tlc creates task; entry deleted
```

### Reconfiguration

```pseudocode
$ nerv config set hooks.builtin.open-questions.enabled false
$ nerv hook list
// disabled built-in still appears in list with status=disabled
```

---

## User Stories

- As a solo dev, I run `nerv init` once and get all built-in
  hooks configured for the AI CLIs I have installed, without
  reading docs for each one.
- As a solo dev, I get an inbox entry when I pivot a session
  with unanswered questions, so I can come back to them later
  instead of losing them.
- As a solo dev, I edit the inbox entry directly (answer
  inline, or check "Promote to tlc") and a single command
  promotes checked items to real tlc tasks.
- As a solo dev, I disable a built-in I don't want via
  `nerv config set hooks.builtin.<name>.enabled false` without
  uninstalling anything; re-enable is a one-liner.
- As a solo dev, I list all configured hooks with `nerv hook
  list` and see both my custom hooks and nerv's built-ins in
  one view, with their enabled/disabled status.
- As a solo dev, I see drift notifications (assignment, topic,
  cwd change, session end) surface as inbox entries, scoped
  to my agent_id (whichever profile is active).

---

## Success Metrics

- Time-to-first-nerv-hook-firing after `nerv init`: < 30s on a
  fresh install.
- Inbox triage workflow round-trip (read → answer → promote):
  < 60s for a typical 3-question entry.
- Zero hook failures on day-to-day work attributable to nerv
  itself (fail-open contract held).
- Cross-CLI consistency: same hook config applies identically
  across all AI CLIs the user has installed.
