/**
 * All 34 nerv event name constants.
 * Generated from spec/events.yaml — do not edit by hand.
 */
export const events = {
  // Session lifecycle
  SessionStart: 'SessionStart',
  SessionEnd: 'SessionEnd',
  Stop: 'Stop',
  StopFailure: 'StopFailure',

  // Prompt lifecycle
  UserPromptSubmit: 'UserPromptSubmit',

  // Tool lifecycle
  PreToolUse: 'PreToolUse',
  PostToolUse: 'PostToolUse',
  PostToolUseFailure: 'PostToolUseFailure',

  // Permission lifecycle
  PermissionRequest: 'PermissionRequest',
  PermissionDenied: 'PermissionDenied',

  // Notification
  Notification: 'Notification',

  // Agent lifecycle
  SubagentStart: 'SubagentStart',
  SubagentStop: 'SubagentStop',
  TeammateIdle: 'TeammateIdle',

  // Task lifecycle
  TaskCreated: 'TaskCreated',
  TaskCompleted: 'TaskCompleted',

  // Context lifecycle
  InstructionsLoaded: 'InstructionsLoaded',
  ConfigChange: 'ConfigChange',
  CwdChanged: 'CwdChanged',
  PreCompact: 'PreCompact',
  PostCompact: 'PostCompact',

  // File lifecycle
  FileChanged: 'FileChanged',

  // Worktree lifecycle
  WorktreeCreate: 'WorktreeCreate',
  WorktreeRemove: 'WorktreeRemove',

  // MCP elicitation lifecycle
  Elicitation: 'Elicitation',
  ElicitationResult: 'ElicitationResult',

  // Extension events (non-Claude CLIs)
  BeforeToolSelection: 'BeforeToolSelection',
  LspDiagnostics: 'LspDiagnostics',
  SessionDiff: 'SessionDiff',
  SessionStatus: 'SessionStatus',

  // Bus / inbox-message lifecycle (kit-bus phase 1)
  // Wire names use snake_case per the canonical naming rule
  // (<container>_<item>_<verb>; see US-0018).
  InboxMessageAdded: 'inbox_message_added',
  InboxMessageUpdated: 'inbox_message_updated',
  InboxMessagePromoted: 'inbox_message_promoted',
  InboxMessageDropped: 'inbox_message_dropped',
} as const;

/** Union type of all nerv event names. */
export type Event = (typeof events)[keyof typeof events];
