package opencode

import (
	"fmt"
	"strings"
)

// pluginTemplate is the TypeScript plugin source that bridges OpenCode
// to nervd. The shim registers handlers for all supported events,
// shells out to nervd with envelope JSON, and throws on block decisions.
//
// Placeholder %s is replaced with the nervd binary path.
const pluginTemplate = `// nerv Bridge Plugin for OpenCode — auto-generated; do not edit.
// Bridges OpenCode's JS/TS plugin model to nervd for language-agnostic hooks.
import { execFileSync } from "child_process";

const NERVD_PATH = "%s";

interface OpenCodeEvent {
  type: string;
  tool?: string;
  input?: unknown;
  output?: unknown;
  error?: string;
  path?: string;
  session_id?: string;
  decision?: string;
}

interface NervDecision {
  action: "allow" | "warn" | "block" | "rewrite";
  message?: string;
  rewrite?: unknown;
  context?: string;
}

// Session-start context, keyed by session id ("" = any session). Pushed
// onto the system prompt by experimental.chat.system.transform on every
// model request, so it persists for the session.
const sessionContext = new Map<string, string>();

function dispatch(event: OpenCodeEvent): NervDecision {
  const envelope = JSON.stringify(event);
  try {
    const result = execFileSync(NERVD_PATH, ["dispatch", "--format", "json"], {
      input: envelope,
      encoding: "utf-8",
      timeout: 10_000,
      maxBuffer: 1024 * 1024,
    });
    return JSON.parse(result.trim());
  } catch (err: unknown) {
    // Fail-open only for transport/execution errors (nervd missing/unreachable).
    const code = (err as { code?: string })?.code;
    if (code === "ENOENT" || code === "EACCES" || code === "EPERM") {
      console.error("[nerv] nervd unreachable, allowing operation:", err);
      return { action: "allow" };
    }
    // Other errors (parse, timeout, etc.) re-throw for visibility.
    throw err;
  }
}

function handleBlocking(event: OpenCodeEvent): void {
  const decision = dispatch(event);
  if (decision.action === "block") {
    throw new Error(decision.message ?? "blocked by nerv hook");
  }
  if (decision.action === "warn" && decision.message) {
    console.warn("[nerv] warning:", decision.message);
  }
  // rewrite: not supported by OpenCode's plugin model (throw-only blocking).
  // Rewrite decisions are logged but not applied.
  if (decision.action === "rewrite") {
    console.warn("[nerv] rewrite decision ignored (not supported by OpenCode)");
  }
}

function handlePassive(event: OpenCodeEvent): void {
  dispatch(event);
}

export default (ctx: unknown) => ({
  // Session lifecycle
  "session.created": async (event: OpenCodeEvent) => {
    const decision = dispatch(event);
    if (decision.action !== "block" && decision.context) {
      sessionContext.set(event.session_id ?? "", decision.context);
    }
  },
  "experimental.chat.system.transform": async (
    input: { sessionID?: string },
    output: { system: string[] },
  ) => {
    for (const key of new Set(["", input.sessionID ?? ""])) {
      const text = sessionContext.get(key);
      if (text) output.system.push(text);
    }
  },
  "session.deleted": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },
  "session.idle": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },
  "session.error": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },
  "session.compacted": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },

  // Tool lifecycle — before is blocking
  "tool.execute.before": async (event: OpenCodeEvent) => {
    handleBlocking(event);
  },
  "tool.execute.after": async (
    event: OpenCodeEvent,
    output?: { output?: string },
  ) => {
    const decision = dispatch({ ...event, type: "tool.execute.after" });
    // Model-context channel: text appended to the tool result the model reads.
    if (
      decision.action !== "block" &&
      decision.context &&
      output &&
      typeof output.output === "string"
    ) {
      output.output += "\n" + decision.context;
    }
  },

  // File lifecycle
  "file.edited": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },

  // Permission lifecycle
  "permission.asked": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },
  "permission.replied": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },

  // TUI
  "tui.prompt.append": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },

  // Todo
  "todo.updated": async (event: OpenCodeEvent) => {
    handlePassive(event);
  },
});
`

// GeneratePlugin returns TypeScript source for the nerv bridge plugin.
// The output is written to .opencode/plugins/nerv-bridge.ts by the
// adapter's Install.
func GeneratePlugin(nervdPath string) (string, error) {
	if nervdPath == "" {
		return "", fmt.Errorf("opencode: nervdPath must not be empty")
	}

	// Escape for safe interpolation into a TS string literal.
	escaped := strings.ReplaceAll(nervdPath, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	escaped = strings.ReplaceAll(escaped, "\n", `\n`)
	escaped = strings.ReplaceAll(escaped, "\r", `\r`)

	return fmt.Sprintf(pluginTemplate, escaped), nil
}
