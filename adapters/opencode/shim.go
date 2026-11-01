package opencode

import (
	"fmt"
	"strings"
)

// pluginTemplate is the TypeScript plugin source that bridges OpenCode
// to nervd. The shim registers handlers for all supported events,
// shells out to nervd with envelope JSON, throws on block decisions, and
// applies rewrite decisions to tool.execute.before args in place.
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

// Session-start context, keyed by session id. Pushed
// onto the system prompt by experimental.chat.system.transform on every
// model request, so it persists for the session.
const sessionContext = new Map<string, string>();

function dispatch(event: OpenCodeEvent): NervDecision {
  const envelope = JSON.stringify(event);
  try {
    const result = execFileSync(NERVD_PATH, ["--adapter", "opencode"], {
      input: envelope,
      encoding: "utf-8",
      timeout: 10_000,
      maxBuffer: 1024 * 1024,
    });
    const decision: NervDecision = JSON.parse(result.trim());
    if (decision.action === "rewrite" && event.type !== "tool.execute.before") {
      console.warn("[nerv] rewrite decision ignored (only tool.execute.before supports rewrite):", event.type);
    }
    return decision;
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

const isPlainObject = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object" && !Array.isArray(v);

// Replace semantics: the rewrite is the new tool input. OpenCode runs the
// tool with the very args object it handed to the hook (session/tools.ts
// reads no other reference), so it is mutated in place, never reassigned.
// v1 does not rename tools or args, so nerv's vocabulary is the host's and
// the rewrite is applied as is.
function applyRewrite(args: unknown, rewrite: unknown): boolean {
  if (!isPlainObject(args) || !isPlainObject(rewrite)) return false;
  for (const k of Object.keys(args)) delete args[k];
  for (const [k, v] of Object.entries(rewrite)) {
    Object.defineProperty(args, k, { value: v, writable: true, enumerable: true, configurable: true });
  }
  return true;
}

function handleBlocking(event: OpenCodeEvent, args?: unknown): void {
  const decision = dispatch(event);
  if (decision.action === "block") {
    throw new Error(decision.message ?? "blocked by nerv hook");
  }
  if ((decision.action === "warn" || decision.action === "rewrite") && decision.message) {
    console.warn("[nerv] warning:", decision.message);
  }
  if (decision.action === "rewrite" && !applyRewrite(args, decision.rewrite)) {
    console.warn("[nerv] rewrite decision ignored (needs an object rewrite and object tool args)");
  }
}

// Completed todo ids already reported, per session, so repeated
// todo.updated snapshots only fire for newly completed items.
const completedTodos = new Map<string, Set<string>>();

function errorText(err: any): string | undefined {
  if (!err) return undefined;
  if (typeof err === "string") return err;
  return err.data?.message ?? err.message ?? err.name;
}

// Maps an OpenCode bus event (the {type, properties} the "event" hook
// receives) to the wire event nervd translates. Returns null for events
// nerv does not map or that must not be reported.
function mapBusEvent(type: string, p: any): OpenCodeEvent | null {
  p = p ?? {};
  const sid: string = p.sessionID ?? p.info?.id ?? "";
  switch (type) {
    case "session.created":
    case "session.deleted":
    case "session.idle":
    case "session.compacted":
      return { type, session_id: sid };
    case "session.error":
      return { type, session_id: sid, error: errorText(p.error) ?? "unknown error" };
    case "file.edited":
      return { type, path: p.file, session_id: sid };
    case "permission.asked":
      return {
        type,
        session_id: sid,
        input: { id: p.id, permission: p.permission, patterns: p.patterns },
      };
    case "permission.replied":
      // Only a rejected request maps (PermissionDenied).
      if (p.reply !== "reject") return null;
      return { type, session_id: sid, decision: "deny" };
    case "tui.prompt.append":
      return { type, input: p.text };
    case "todo.updated": {
      const seen = completedTodos.get(sid) ?? new Set<string>();
      const fresh = (p.todos ?? []).filter(
        (t: any) => t?.status === "completed" && !seen.has(t.id),
      );
      if (fresh.length === 0) return null;
      for (const t of fresh) seen.add(t.id);
      completedTodos.set(sid, seen);
      return { type, session_id: sid, input: fresh };
    }
    default:
      return null;
  }
}

export default (_ctx: unknown) => ({
  // Every bus event (session.*, file.edited, permission.*, todo.updated,
  // tui.prompt.append) arrives through this one hook as
  // { event: { type, properties } }; OpenCode plugin API v1 has no
  // per-event top-level keys for them. It is called fire-and-forget, so
  // errors are logged, never thrown.
  event: async ({ event }: { event: { type: string; properties?: any } }) => {
    try {
      const oe = mapBusEvent(event.type, event.properties);
      if (!oe) return;
      const decision = dispatch(oe);
      const sid = oe.session_id ?? "";
      if (event.type === "session.created") {
        if (decision.action !== "block" && decision.context) {
          sessionContext.set(sid, decision.context);
        }
      } else if (event.type === "session.deleted") {
        sessionContext.delete(sid);
        completedTodos.delete(sid);
      }
    } catch (err) {
      console.error("[nerv] event hook failed:", event.type, err);
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

  // Tool lifecycle. Throwing from tool.execute.before aborts the tool
  // call (plugin.trigger does not catch); the message reaches the model.
  // A rewrite decision replaces output.args in place.
  "tool.execute.before": async (
    input: { tool?: string; sessionID?: string },
    output?: { args?: unknown },
  ) => {
    handleBlocking(
      {
        type: "tool.execute.before",
        tool: input.tool,
        session_id: input.sessionID,
        input: output?.args,
      },
      output?.args,
    );
  },
  "tool.execute.after": async (
    input: { tool?: string; sessionID?: string; args?: unknown },
    output?: { output?: string },
  ) => {
    const decision = dispatch({
      type: "tool.execute.after",
      tool: input.tool,
      session_id: input.sessionID,
      input: input.args,
      output: output?.output,
    });
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
});
`

// GeneratePlugin returns TypeScript source for the nerv bridge plugin.
// The output is written to .opencode/plugins/nerv-bridge.ts by the
// adapter's Install.
func GeneratePlugin(nervdPath string) (string, error) {
	if nervdPath == "" {
		return "", fmt.Errorf("opencode: nervdPath must not be empty")
	}

	return fmt.Sprintf(pluginTemplate, escapeTS(nervdPath)), nil
}

// escapeTS escapes s for safe interpolation into a TS string literal.
func escapeTS(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return strings.ReplaceAll(s, "\r", `\r`)
}
