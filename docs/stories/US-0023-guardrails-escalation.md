# US-0023: Guardrails escalate to human or peers when policy demands

**Persona:** Platform Engineer (rule author), Solo Developer (reviewer)
**Priority:** P2 (HITL: P1; manager + peer-N: P2)

## Story

As a platform engineer setting policy on a shared repo, I want
risky-but-not-prohibited operations (e.g. `git reset --hard`,
`git clean -fd`, `rm -rf *`) to ask the operator before they run,
or — for higher-stakes operations — wait for one or more peer
approvals via the bus inbox. Per-rule mode override in
`hooks.yaml` should let me dial each rule between
`allow|block|rewrite|hitl|manager|peer-N`.

## Acceptance Criteria

- [ ] Given a rule with `default_mode: hitl`, when the rule fires,
      then guardrails prompts the user via stderr with the rule
      reason + the proposed command + Y/n; the daemon blocks the
      decision until the operator replies
- [ ] Given a rule with `mode: manager`, when the rule fires, then
      guardrails publishes an `inbox_message_added` event with
      `kind="approval-request"` routed to the manager profile
      (resolved via `aps`); the daemon returns a deferred decision
      reference; final decision is set when an
      `inbox_message_promoted` event with the matching ref lands
- [ ] Given a rule with `mode: peer-N` (N ≥ 1), when the rule
      fires, then guardrails publishes the same approval-request
      payload to N peer profiles; the rule resolves to Allow when
      N approvals land OR Block when any peer denies OR after
      `async_timeout_hours` (default 24h) with no quorum
- [ ] Given the user types Y at the hitl prompt, then the rule
      resolves to Allow + backup-if-destructive; given the user
      types n (or anything not Y), then Block
- [ ] Given a hitl rule with `destructive: true`, when the user
      approves, then a backup archive lands on disk before the
      Allow decision flows out

## Out of scope (v1)

- Routing approval requests to anything other than the bus inbox
  (no Slack / Webhook integration in v1)
- Asymmetric quorum (e.g. "any of these 3 OR all of these 2")
- Approval audit trail beyond the inbox event log

## Acceptance test path

- `hitl_test.go`: stdin-mock + stderr-capture; verify
  blocking until reply
- `escalate_test.go`: bus-fake (memory adapter) +
  in-process Resolve poll; verify ref returned, approval flow
- e2e covers all three modes end-to-end

## Telemetry

- `inbox_message_added` per escalation
- `inbox_message_promoted` per approval
- `inbox_message_dropped` per timeout / denial
