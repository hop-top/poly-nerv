"""All 34 nerv event name constants.

Generated from spec/events.yaml -- do not edit by hand.
"""

# Session lifecycle
SESSION_START = "SessionStart"
SESSION_END = "SessionEnd"
STOP = "Stop"
STOP_FAILURE = "StopFailure"

# Prompt lifecycle
USER_PROMPT_SUBMIT = "UserPromptSubmit"

# Tool lifecycle
PRE_TOOL_USE = "PreToolUse"
POST_TOOL_USE = "PostToolUse"
POST_TOOL_USE_FAILURE = "PostToolUseFailure"

# Permission lifecycle
PERMISSION_REQUEST = "PermissionRequest"
PERMISSION_DENIED = "PermissionDenied"

# Notification
NOTIFICATION = "Notification"

# Agent lifecycle
SUBAGENT_START = "SubagentStart"
SUBAGENT_STOP = "SubagentStop"
TEAMMATE_IDLE = "TeammateIdle"

# Task lifecycle
TASK_CREATED = "TaskCreated"
TASK_COMPLETED = "TaskCompleted"

# Context lifecycle
INSTRUCTIONS_LOADED = "InstructionsLoaded"
CONFIG_CHANGE = "ConfigChange"
CWD_CHANGED = "CwdChanged"
PRE_COMPACT = "PreCompact"
POST_COMPACT = "PostCompact"

# File lifecycle
FILE_CHANGED = "FileChanged"

# Worktree lifecycle
WORKTREE_CREATE = "WorktreeCreate"
WORKTREE_REMOVE = "WorktreeRemove"

# MCP elicitation lifecycle
ELICITATION = "Elicitation"
ELICITATION_RESULT = "ElicitationResult"

# Extension events (non-Claude CLIs)
BEFORE_TOOL_SELECTION = "BeforeToolSelection"
LSP_DIAGNOSTICS = "LspDiagnostics"
SESSION_DIFF = "SessionDiff"
SESSION_STATUS = "SessionStatus"

# Bus / inbox-message lifecycle (kit-bus phase 1)
# Wire names use snake_case per the canonical naming rule
# (<container>_<item>_<verb>; see US-0018).
INBOX_MESSAGE_ADDED = "inbox_message_added"
INBOX_MESSAGE_UPDATED = "inbox_message_updated"
INBOX_MESSAGE_PROMOTED = "inbox_message_promoted"
INBOX_MESSAGE_DROPPED = "inbox_message_dropped"


# Convenience collection: all event names as a frozenset.
EVENTS: frozenset[str] = frozenset([
    SESSION_START, SESSION_END, STOP, STOP_FAILURE,
    USER_PROMPT_SUBMIT,
    PRE_TOOL_USE, POST_TOOL_USE, POST_TOOL_USE_FAILURE,
    PERMISSION_REQUEST, PERMISSION_DENIED,
    NOTIFICATION,
    SUBAGENT_START, SUBAGENT_STOP, TEAMMATE_IDLE,
    TASK_CREATED, TASK_COMPLETED,
    INSTRUCTIONS_LOADED, CONFIG_CHANGE, CWD_CHANGED,
    PRE_COMPACT, POST_COMPACT,
    FILE_CHANGED,
    WORKTREE_CREATE, WORKTREE_REMOVE,
    ELICITATION, ELICITATION_RESULT,
    BEFORE_TOOL_SELECTION, LSP_DIAGNOSTICS,
    SESSION_DIFF, SESSION_STATUS,
    INBOX_MESSAGE_ADDED, INBOX_MESSAGE_UPDATED,
    INBOX_MESSAGE_PROMOTED, INBOX_MESSAGE_DROPPED,
])


def events() -> frozenset[str]:
    """Return the set of all valid nerv event names."""
    return EVENTS
