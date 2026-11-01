package opencode

import (
	"fmt"
)

// pluginTemplateV2 is the TypeScript plugin source that bridges OpenCode
// plugin API v2 (>=2.0.0) to nervd. It is a plain default-exported
// definition ({id, setup}); Plugin.define is the identity function, so no
// import of @opencode/plugin is needed and the file resolves from any
// project directory.
//
// V2 differences absorbed here, so translate.go and the nervd wire shape
// stay identical to the v1 bridge:
//   - hooks register in setup() via ctx.tool.hook / ctx.session.hook;
//   - bus events arrive through ctx.event.subscribe() as {type, data};
//   - tool ids differ (shell, subagent) and path args are named "path";
//   - there is no todo.updated / file.edited / session.idle bus event, so
//     they are derived from todowrite/edit/write/patch completions and
//     session.execution.* events;
//   - a session's first sighting may be its first model request rather
//     than a session.created event, so session start is dispatched once
//     per session id from either source.
//
// Placeholder %s is replaced with the nervd binary path.
const pluginTemplateV2 = `// nerv Bridge Plugin for OpenCode (plugin API v2) — auto-generated; do not edit.
// Bridges OpenCode's plugin model to nervd for language-agnostic hooks.
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

// Session-start context, keyed by session id. Pushed onto the system
// prompt by the session "context" hook on every model request.
const sessionContext = new Map<string, string>();
const started = new Set<string>();

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

// v2 tool ids -> the v1 names nerv's synthesized-event rules match on.
const TOOL_NAMES: Record<string, string> = { shell: "bash", subagent: "task" };
const toolName = (id: string): string => TOOL_NAMES[id] ?? id;

// v2 names the file argument "path"; v1 (and nerv) use "filePath".
function normInput(input: any): unknown {
  if (input && typeof input === "object" && typeof input.path === "string" && input.filePath === undefined) {
    return { ...input, filePath: input.path };
  }
  return input;
}

const isPlainObject = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object" && !Array.isArray(v);

// True when normInput added the filePath alias, i.e. the host names the
// file argument "path" and a rewrite must be mapped back to it.
const aliasedPath = (input: any): boolean =>
  isPlainObject(input) && typeof input.path === "string" && input.filePath === undefined;

// Inverse of normInput: a rewrite arrives in nerv's vocabulary (filePath);
// the host reads "path". Keys normInput did not touch pass through.
// Precedence when the rewrite carries both: filePath wins, unless it is
// still the original value and path was changed (a handler that edited
// only path), in which case path wins.
function denormRewrite(rewrite: Record<string, unknown>, orig: any): Record<string, unknown> {
  if (!aliasedPath(orig) || rewrite.filePath === undefined) return rewrite;
  const { filePath, ...rest } = rewrite;
  const path =
    rest.path !== undefined && filePath === orig.path && rest.path !== orig.path ? rest.path : filePath;
  return { ...rest, path };
}

// Replace semantics: the rewrite is the new tool input. The hook's input
// object is mutated in place (cleared, then assigned) so every holder of
// the reference sees it; a non-object input is replaced outright.
function applyRewrite(event: any, rewrite: unknown): boolean {
  if (!isPlainObject(rewrite)) return false;
  const next = denormRewrite(rewrite, event.input);
  const cur = event.input;
  if (!isPlainObject(cur)) {
    event.input = next;
    return true;
  }
  for (const k of Object.keys(cur)) delete cur[k];
  for (const [k, v] of Object.entries(next)) {
    Object.defineProperty(cur, k, { value: v, writable: true, enumerable: true, configurable: true });
  }
  return true;
}

// Joins the text parts of a tool result into the string the v1 bridge sent.
function resultText(result: any): string | undefined {
  const c = result?.content;
  if (typeof c === "string") return c;
  if (Array.isArray(c)) {
    return c
      .filter((p: any) => p?.type === "text" && typeof p.text === "string")
      .map((p: any) => p.text)
      .join("\n");
  }
  return undefined;
}

// Model-context channel: text appended to the tool result the model reads.
function appendContext(result: any, text: string): void {
  if (!result) return;
  const c = result.content;
  if (typeof c === "string") result.content = c + "\n" + text;
  else if (Array.isArray(c)) c.push({ type: "text", text });
  else result.content = [{ type: "text", text }];
}

// A session is announced to nervd once, from session.created or from the
// first model request, whichever OpenCode shows first.
function startSession(sid: string): void {
  if (started.has(sid)) return;
  started.add(sid);
  const decision = dispatch({ type: "session.created", session_id: sid });
  if (decision.action !== "block" && decision.context) {
    sessionContext.set(sid, decision.context);
  }
}

function endSession(sid: string): void {
  dispatch({ type: "session.deleted", session_id: sid });
  sessionContext.delete(sid);
  started.delete(sid);
  completedTodos.delete(sid);
}

// Completed todo ids already reported, per session, so repeated
// todowrite snapshots only fire for newly completed items.
const completedTodos = new Map<string, Set<string>>();

function reportTodos(sid: string, todos: any[]): void {
  const seen = completedTodos.get(sid) ?? new Set<string>();
  const fresh = todos.filter((t: any) => {
    if (t?.status !== "completed") return false;
    const key = t.id ?? t.content;
    return !seen.has(key);
  });
  if (fresh.length === 0) return;
  for (const t of fresh) seen.add(t.id ?? t.content);
  completedTodos.set(sid, seen);
  dispatch({ type: "todo.updated", session_id: sid, input: fresh });
}

// Files a completed edit/write/patch call touched.
function editedFiles(tool: string, input: any, result: any): string[] {
  if (tool === "edit" || tool === "write") {
    return typeof input?.path === "string" ? [input.path] : [];
  }
  if (tool === "patch") {
    const applied = result?.output?.applied;
    if (!Array.isArray(applied)) return [];
    return applied.map((a: any) => a?.target ?? a?.resource).filter((p: any) => typeof p === "string");
  }
  return [];
}

// Maps a v2 bus event ({type, data}) to the wire event nervd translates.
// Returns null for events nerv does not map or that must not be reported.
function mapBusEvent(type: string, d: any): OpenCodeEvent | null {
  d = d ?? {};
  const sid: string = d.sessionID ?? "";
  switch (type) {
    case "session.execution.succeeded":
      return { type: "session.idle", session_id: sid };
    case "session.execution.interrupted":
      if (d.reason === "shutdown") return null;
      return { type: "session.idle", session_id: sid };
    case "session.execution.failed":
      return { type: "session.error", session_id: sid, error: d.error?.message ?? d.error?.type ?? "unknown error" };
    case "session.compaction.ended":
      return { type: "session.compacted", session_id: sid };
    case "permission.asked":
      return {
        type: "permission.asked",
        session_id: sid,
        input: { id: d.id, permission: d.action, patterns: d.resources },
      };
    case "permission.replied":
      // Only a rejected request maps (PermissionDenied).
      if (d.reply !== "reject") return null;
      return { type: "permission.replied", session_id: sid, decision: "deny" };
    case "tui.prompt.append":
      return { type, input: d.text };
    default:
      return null;
  }
}

const BUS_TYPES = [
  "session.created",
  "session.deleted",
  "session.execution.succeeded",
  "session.execution.interrupted",
  "session.execution.failed",
  "session.compaction.ended",
  "permission.asked",
  "permission.replied",
  "tui.prompt.append",
];

function onBusEvent(ev: { type: string; data?: any }): void {
  if (ev.type === "session.created") {
    startSession(ev.data?.sessionID ?? "");
    return;
  }
  if (ev.type === "session.deleted") {
    endSession(ev.data?.sessionID ?? "");
    return;
  }
  const oe = mapBusEvent(ev.type, ev.data);
  if (oe) dispatch(oe);
}

export default {
  id: "nerv.bridge",
  async setup(ctx: any) {
    // Bus events (session lifecycle, permissions, prompt). Dispatch errors
    // are logged, never thrown: the stream is fire-and-forget.
    const abort = new AbortController();
    void (async () => {
      try {
        for await (const ev of ctx.event.subscribe({ types: BUS_TYPES, signal: abort.signal })) {
          try {
            onBusEvent(ev as { type: string; data?: any });
          } catch (err) {
            console.error("[nerv] event hook failed:", (ev as any)?.type, err);
          }
        }
      } catch (err) {
        if (!abort.signal.aborted) console.error("[nerv] event stream failed:", err);
      }
    })();

    // Session-start context onto the system prompt, on every model request.
    await ctx.session.hook("context", (event: any) => {
      const sid: string = event.sessionID ?? "";
      try {
        startSession(sid);
      } catch (err) {
        console.error("[nerv] session start failed:", sid, err);
      }
      const text = sessionContext.get(sid);
      if (text) event.system.push({ type: "text", text });
    });

    // Throwing from execute.before rejects the call; the message reaches
    // the model. A rewrite decision replaces event.input.
    await ctx.tool.hook("execute.before", (event: any) => {
      const decision = dispatch({
        type: "tool.execute.before",
        tool: toolName(event.tool),
        session_id: event.sessionID,
        input: normInput(event.input),
      });
      if (decision.action === "block") {
        throw new Error(decision.message ?? "blocked by nerv hook");
      }
      if ((decision.action === "warn" || decision.action === "rewrite") && decision.message) {
        console.warn("[nerv] warning:", decision.message);
      }
      if (decision.action === "rewrite" && !applyRewrite(event, decision.rewrite)) {
        console.warn("[nerv] rewrite decision ignored (needs an object rewrite)");
      }
    });

    await ctx.tool.hook("execute.after", (event: any) => {
      const tool = toolName(event.tool);
      const failed = event.status === "error";
      const decision = dispatch({
        type: "tool.execute.after",
        tool,
        session_id: event.sessionID,
        input: normInput(event.input),
        output: failed ? undefined : resultText(event.result),
        error: failed ? (event.error?.message ?? "tool failed") : undefined,
      });
      if (failed) return;
      if (decision.action !== "block" && decision.context) {
        appendContext(event.result, decision.context);
      }
      // Events v1 gets from the bus but v2 only implies.
      for (const file of editedFiles(event.tool, event.input, event.result)) {
        dispatch({ type: "file.edited", path: file, session_id: event.sessionID });
      }
      if (event.tool === "todowrite") {
        const todos = event.result?.output?.todos ?? event.input?.todos;
        if (Array.isArray(todos)) reportTodos(event.sessionID, todos);
      }
    });

    return () => abort.abort();
  },
};
`

// GeneratePluginV2 returns TypeScript source for the nerv bridge plugin
// targeting OpenCode plugin API v2. The output is written to
// .opencode/nerv-bridge/index.ts by the adapter's install.
func GeneratePluginV2(nervdPath string) (string, error) {
	if nervdPath == "" {
		return "", fmt.Errorf("opencode: nervdPath must not be empty")
	}
	return fmt.Sprintf(pluginTemplateV2, escapeTS(nervdPath)), nil
}
