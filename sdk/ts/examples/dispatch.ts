/**
 * Minimal example: dispatch a synthetic PreToolUse envelope at a
 * running nerv daemon and print the merged decision.
 *
 * Run:
 *   tsx examples/dispatch.ts                 # uses defaultSocketPath()
 *   tsx examples/dispatch.ts /tmp/nerv.sock  # explicit socket path
 */
import { create } from "@bufbuild/protobuf";
import {
  NervClient,
  EnvelopeSchema,
  DispatchRequestSchema,
  Action,
  defaultSocketPath,
} from "../src/index.js";

const socket = process.argv[2] ?? defaultSocketPath();
const client = new NervClient({ socket });

try {
  const envelope = create(EnvelopeSchema, {
    nervVersion: "1.0.0-draft",
    event: "PreToolUse",
    eventId: crypto.randomUUID(),
    payload: {
      tool_name: "Bash",
      tool_input: { command: "echo hello" },
    },
  });

  const req = create(DispatchRequestSchema, { envelope });
  const res = await client.events.dispatch(req);

  const action = res.decision ? Action[res.decision.action] : "no-decision";
  const message = res.decision?.message ?? "";
  console.log(JSON.stringify({ action, message }, null, 2));
} finally {
  client.close();
}
