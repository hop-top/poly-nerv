# Persona: Automation Builder (nerv Extension)

**Primary Role:** Power user who writes custom hooks AND
authors built-ins for promotion into nerv itself. Lives in the
intersection between user-installable hooks (`nerv install
<hook>`) and contributor patches (a built-in shipped in the
binary).

---

## Base Goals

- Express recurring guardrails as code, not as wiki docs.
- Promote successful per-machine hooks to team-wide defaults
  via the built-ins path.
- Iterate fast: write hook → test against fixtures → ship.

## Base Pain Points

- Custom hook handlers fork-exec per event — slow for high-
  frequency events. Want in-process when latency matters.
- No clean path from "experimental shell-script hook" to "Go
  built-in shipped with nerv." Today this is a fork-and-PR
  expedition.
- Bus events from one hook can't easily feed another hook's
  logic. Currently must marshal through user code.

---

## nerv Interaction Patterns

### Authoring a built-in

```pseudocode
// internal/hookpkg/builtins/myrule/builtin.go
var Def = builtins.BuiltinDef{
    Name:          "my-rule",
    SchemaVersion: 1,
    Events:        []string{"PreToolUse"},
    Func:          Handle,
}

// register in internal/hookpkg/builtins/registry.go
var All = []BuiltinDef{
    openquestions.Def,
    myrule.Def,  // add here
}

// in spec/builtins.yaml, mirror metadata for the wizard
- id: my-rule
  description: ...
  default_enabled: false
```

### Hook composition via bus

```pseudocode
// my-rule subscribes to events emitted by another hook
sub := bus.Subscribe("inbox_message_added")
for event := range sub {
    if event.payload.kind == "open_question" {
        // act on it: file ticket, ping channel, etc.
    }
}
```

### Iterating on a custom hook

```pseudocode
$ nerv testhook ./my-hook.sh --event PreToolUse --fixture fixture.json
// dry-run against a captured envelope; prints decision
```

---

## User Stories

- As an automation builder, I author a new nerv built-in by
  adding a `BuiltinDef` to a Go package and one line to
  `builtins.All`, then ship a PR — no wizard plumbing needed
  beyond updating `spec/builtins.yaml`.
- As an automation builder, I subscribe my custom hook to
  another hook's bus events so I can compose behaviour
  without re-implementing detection in my own hook.
- As an automation builder, I test my hook against captured
  envelope fixtures via `nerv testhook` so I never need to
  fire a real AI CLI session to iterate.
- As an automation builder, I see my custom hook AND
  built-ins together in `nerv hook list` so I can verify the
  full event-routing tree at a glance.
- As an automation builder, I emit my own bus events from a
  custom hook (with appropriately namespaced event names —
  `<my-tool>_<entity>_<verb>`) so future tools can subscribe
  without me coordinating with them.

---

## Success Metrics

- Time-to-first-built-in PR after reading the
  `builtins-registry.md` ADR: < 1 hour for a Go-fluent
  contributor.
- Cross-hook composition via bus: zero file-watching, zero
  polling — pub/sub round-trip < 50ms for memory adapter.
- `nerv testhook` round-trip per fixture: < 500ms (fork-exec
  bound for shell hooks; in-process for built-ins).
- Built-in metadata schema mismatch detection: 100% of YAML
  entries with no matching `builtins.All.Name` rejected at
  daemon startup via `builtins.Validate()`.
