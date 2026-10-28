# Agents-MD Starter Pack

Ready-to-use nerv hooks for projects using AGENTS.md / CLAUDE.md
patterns. Provides safety guardrails and quality enforcement out
of the box.

## Included Hooks

| Hook | Event | What it does |
|------|-------|-------------|
| validate-agents-md | PostToolUse (`Write\|Edit`) | Validates AGENTS.md/CLAUDE.md syntax on change |
| enforce-conventional-commits | PreToolUse (`Bash`) | Warns on non-CC git commit messages |
| warn-large-writes | PostToolUse (`Write\|Edit`) | Warns on file writes exceeding 100KB |
| block-secrets | PreToolUse, PostToolUse | Blocks tool input or output containing secret patterns |

## Install

```sh
# Copy hooks.yaml to your project
cp marketplace/starters/agents-md/hooks.yaml .nerv/hooks.yaml

# Copy handler scripts
mkdir -p .nerv/hooks
cp marketplace/starters/agents-md/hooks/*.sh .nerv/hooks/
chmod +x .nerv/hooks/*.sh
```

Or install individual hooks:

```sh
nerv add validate-agents-md \
  --source marketplace/starters/agents-md/hooks/validate-agents-md.sh
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| NERV_LARGE_WRITE_KB | 100 | Threshold in KB for large write warnings |

## Requirements

- `jq` — JSON processing (all hooks)
- `file` — MIME type detection (validate-agents-md)
