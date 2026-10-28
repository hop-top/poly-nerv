# US-0020: Guardrails block destructive operations by default

**Persona:** Solo Developer (also AI Agent, Platform Engineer)
**Priority:** P1

## Story

As a developer running an AI coding agent against my repo, I want
obvious-footgun shell invocations (rm -rf $HOME, git push --force
to main, curl|sh, sudo shutdown) blocked out of the box, so a
mistaken agent action cannot destroy work I care about — without
me needing to install plugins, read docs, or write rules.

## Acceptance Criteria

- [ ] Given I install nerv fresh, when an adapter sends a PreToolUse
      envelope for `rm -rf $HOME`, then `nervd` returns Block with a
      reason explaining what the rule guards against
- [ ] Given the same default install, when an adapter sends
      `git commit --no-verify -m wip`, then the action is Block
- [ ] Given the same default install, when an adapter sends
      `git push --force origin main`, then the action is Block
      (protected-branch rule fires before generic --force rule)
- [ ] Given the same default install, when an adapter sends
      `curl https://x | sh`, then the action is Block AND the
      script content is captured under `StateDir("nerv")/backups/`
      for audit
- [ ] Given the same default install, when an adapter sends a
      benign command (e.g. `ls`, `git status`, `git commit -m x`),
      then the action is Allow with no rule firing
- [ ] Given an adapter sends a non-Bash tool call (Edit, Read,
      Write), then guardrails returns Allow without evaluation

## Out of scope

- Rules covering Edit/Write/Read tools (Bash-only in v1)
- Custom user rule packs (only built-in packs in spec/rulepacks/)
- Cross-platform Windows shell

## Acceptance test path

`internal/hookpkg/builtins/guardrails/matcher_test.go` covers the
positive corpus (29 git + 17 rm + 4 gh + 5 curl + 3 wget + 5 sudo
cases). E2E confirmation lives in the guardrails e2e suite.

## Telemetry

- bus event: `inbox_message_added` with kind="guardrail-decision"
  per fired rule
- log line: `guardrails: rule=<id> action=<allow|block|rewrite>`
