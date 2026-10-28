# nerv .agents/ plugin

Canonical source for the `nerv` skill-spec plugin: one unified skill
covering the whole Universal Hooks Protocol surface plus one slash
command per user-facing CLI verb. Wraps the `nerv` CLI for managing
AI hooks across Claude Code, Codex, Gemini, OpenCode, and the rest
of the supported adapter matrix.

`.agents/` is host-agnostic: the skill and command files follow open
specs, and the Claude Code plugin manifest lives at
`.claude-plugin/plugin.json`.

## What this plugin gives you

nerv is the Universal Hooks Protocol — one hook definition, one event
taxonomy, one handler contract, every supported AI coding CLI. The
plugin exposes nerv's surface to agents as:

- **One skill** — `.agents/skills/nerv/SKILL.md` — covering the
  hooks / events / decisions model, the `nervd` daemon, adapters,
  envelope wire format, `.nerv/` project state, and the lifecycle of
  authoring, installing, testing, replaying, and disabling hooks.
- **Seven user-facing slash commands** — `.agents/commands/nerv-*.md`
  — one per shipped CLI verb. Each command's "Related skill" footer
  points back at the single skill.

### Command index

| Command | Wraps | Status |
|---|---|---|
| `/nerv-add` | `nerv add` | stable |
| `/nerv-create` | `nerv create` | stable |
| `/nerv-disable` | `nerv disable` / `nerv enable` | stable |
| `/nerv-doctor` | `nerv doctor` | stable |
| `/nerv-install` | `nerv install add [cli]` | stable |
| `/nerv-replay` | `nerv replay` | stable |
| `/nerv-test` | `nerv test [hook]` | stable |

## Quick start

Install the `nerv` CLI (available after the first tagged release):

```sh
go install hop.top/nerv/cmd/nerv@latest
go install hop.top/nerv/cmd/nervd@latest
nerv install add        # no CLI name = every detected CLI
nerv daemon start       # spin up nervd
nerv doctor             # verify health
```

Then load the plugin into Claude Code from a checkout of this repo:

```sh
claude --plugin-dir .agents
```

The plugin manifest at `.claude-plugin/plugin.json` is what Claude
Code reads.

## Layout

```
.agents/
├── .claude-plugin/
│   └── plugin.json                — Claude Code plugin manifest
├── skills/
│   └── nerv/
│       └── SKILL.md               — single unified skill
├── commands/
│   ├── nerv-add.md                — add a hook
│   ├── nerv-create.md             — LLM-generated hook
│   ├── nerv-disable.md            — disable hooks/events/bindings
│   ├── nerv-doctor.md             — health check + --fix
│   ├── nerv-install.md            — install per-CLI adapter
│   ├── nerv-replay.md             — replay a recorded event
│   └── nerv-test.md               — run hook with synthetic envelope
├── references/
│   ├── hooks-yaml-schema.md       — hooks.yaml field reference
│   ├── event-catalog.md           — 36 canonical events
│   └── envelope-shape.md          — wire format details
└── README.md                      — this file
```

The unified `SKILL.md` follows the
[Agent Skills Open Standard](https://agentskills.io/specification) —
YAML frontmatter (`name`, `description`, optional `license` and
`metadata`) plus a markdown body. Long-form material lives in sibling
`references/` files so the skill body stays scannable.

Each `commands/nerv-*.md` follows the
[Claude Code slash-command spec](https://code.claude.com/docs/en/plugins-reference)
— `description`, `argument-hint`, `allowed-tools`.

The same `SKILL.md` renders natively in
[Codex CLI Skills](https://developers.openai.com/codex/skills) and in
Gemini CLI's skills slot.

## Install per host

### Claude Code

Load `.agents/` as a plugin directory (`claude --plugin-dir .agents`);
Claude Code reads the manifest at `.claude-plugin/plugin.json`.

### Codex CLI and Gemini CLI

Point the host's skills directory at `.agents/skills/` (copy or link
`.agents/skills/nerv/`); the same `SKILL.md` renders natively.

## Related

- Main repo README: [`../README.md`](../README.md)
- Upstream issues / questions: <https://github.com/hop-top/poly-nerv>

## Sources

- [Agent Skills Open Standard](https://agentskills.io/specification)
- [anthropics/skills reference repository](https://github.com/anthropics/skills)
- [Claude Code Plugins reference](https://code.claude.com/docs/en/plugins-reference)
- [Codex CLI Agent Skills](https://developers.openai.com/codex/skills)
