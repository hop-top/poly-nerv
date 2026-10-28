# US-0022: Guardrails capture a backup before destructive ops execute

**Persona:** Solo Developer, Platform Engineer
**Priority:** P1

## Story

As a developer who occasionally allows destructive operations
through guardrails (e.g. `git reset --hard` after a deliberate
hitl approval), I want the prior state captured to disk first so
I can recover if the destruction was a mistake. The backup must
not depend on me remembering to stash.

## Acceptance Criteria

- [ ] Given a rule with `destructive: true` and `backup_strategy:
      <one of: path_archive, git_stash_full, git_untracked,
      remote_bundle, script_capture>`, when the rule's resolved
      action is not Block, then guardrails captures an archive at
      `StateDir("nerv")/backups/<projectKey>/<UTC-ts>-<ruleID>.tar.gz`
      before returning the decision
- [ ] Given the backup capture fails (disk full, permission
      denied, command not found, etc.), then guardrails upgrades
      the decision to Block with reason "backup failed: <err>"
      so the destruction does not proceed silently
- [ ] Given a `path_archive` strategy, when the command is
      `rm -rf <path>`, then the archive contains every file that
      existed under `<path>` at capture time (modulo skipped
      `StateDir/backups/` recursion guard)
- [ ] Given a `git_untracked` strategy, when the command is
      `git clean -fd`, then the archive contains the union of
      `git ls-files --others --exclude-standard` at capture time
- [ ] Given a `script_capture` strategy, when the command is
      `curl https://x | sh`, then the archive contains the
      fetched script bytes alongside the URL (so post-mortem can
      verify what would have run)
- [ ] Given two worktrees of the same bare repo trigger backups,
      when both are captured, then both archives land under the
      same `<projectKey>` directory so retention applies per repo,
      not per worktree

## Out of scope

- Auto-restore (manual `tar xzf` for v1)
- Backup of remote-side state beyond the bundled ref (no GitHub
  API integration)
- Encrypted archives (filesystem-level encryption only)

## Acceptance test path

- `backup_test.go` (6 cases)
- `projectkey_test.go::TestProjectKey_Worktree`

## Telemetry

- archive path returned in `BackupResult.Path`
- size + ctime in `BackupResult.{Bytes,Created}` for retention GC
