# US-0012: Team rollout via wizard preset

**Persona:** Platform Engineer
**Priority:** P1

## Story

As a platform engineer rolling nerv out to a team, I want
to write a single `team-defaults.yaml` describing exactly
which built-ins should be enabled and with what config, then
distribute it so engineers can run `nerv init --from
team-defaults.yaml --yes` and get identical setup in one
command.

## Acceptance Criteria

- [ ] Given a preset YAML at `team-defaults.yaml` declaring
      `{open-questions: enabled, <other>: disabled}`, when I
      run `nerv init --from team-defaults.yaml --yes`, then
      the preset is applied silently AND a JSON summary is
      written to stdout
- [ ] Given the same preset is applied on two engineers'
      machines, when both run `nerv hook list --format json`
      after init, then the output is byte-identical for
      built-in entries (modulo `detected_cli` which is
      machine-local)
- [ ] Given a preset references a built-in not in
      `builtins.All` (typo, version skew), when wizard runs,
      then `Validate()` aborts with a clear error before
      writing anything
- [ ] Given a preset is malformed YAML, when wizard runs,
      then it exits with a parse error pointing at the bad
      line — does NOT silently apply partial state
- [ ] Given preset and `--reset` are both passed, when
      wizard runs, then `wizard-state.yaml.seen_builtins` is
      cleared first, then preset applied — useful for
      forcibly re-baselining a team after policy change
- [ ] Given `--yes` and `--from` are both passed, when
      wizard runs, then it errors in `PreRunE` (mutually
      exclusive)

## Notes

- Preset is ALSO valid for `nerv init --from preset.yaml`
  WITHOUT `--yes` — interactive mode pre-fills choices,
  user can override.
- Useful for team-wide rollouts AND for reproducing a known-
  good setup on a new machine.
