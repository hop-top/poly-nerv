# Aider CLI — nerv Adapter Research

## Overview

Aider (https://aider.chat) is a Python-based AI coding assistant CLI.
Binary: `aider`. Config: `.aider.conf.yml` + `~/.aider.conf.yml` (YAML).

## Hook System

Aider has **no native hook/event system**. It does not emit structured
events that external tools can subscribe to.

### Integration Points

Aider provides two config-driven command hooks:

- `lint-cmd` — runs after code changes, before commit
- `test-cmd` — runs after code changes, before commit
- `auto-lint: true` — enables automatic lint-cmd execution
- `auto-test: true` — enables automatic test-cmd execution

These are the only extensibility points available for nerv integration.
The nerv bridge uses these to synthesize PreToolUse/PostToolUse events.

### Config Format

YAML file at `.aider.conf.yml` (project) or `~/.aider.conf.yml` (global).

```yaml
lint-cmd: nerv dispatch aider lint
test-cmd: nerv dispatch aider test
auto-lint: true
auto-test: true
```

## Event Mapping

| nerv Event     | Aider Source | Notes                           |
|---------------|-------------|----------------------------------|
| SessionStart  | bridge      | Synthesized from process start   |
| SessionEnd    | bridge      | Synthesized from process exit    |
| PreToolUse    | lint-cmd    | Synthesized from lint invocation |
| PostToolUse   | test-cmd    | Synthesized from test invocation |
| FileChanged   | bridge      | Synthesized from file detection  |

## Detection

- Binary: `aider --version`
- Config: check `.aider.conf.yml` or `~/.aider.conf.yml` existence

## Limitations

- No real-time event stream; only lint/test command hooks
- No blocking/rewrite capability for tool calls
- No subagent, task, or permission events
- Session events require external process monitoring
- All events are synthesized; none are native

## References

- https://aider.chat/docs/config/aider_conf.html
- https://aider.chat/docs/config/options.html
