// Parity client for the TypeScript SDK.
//
// Reads NERV_SOCKET from env, dials the daemon, calls List then Dispatch
// against the canonical parity contract, and prints both markers on
// stdout (list event name + dispatch decision message). The Go harness
// (TestParity_TypeScript) asserts stdout contains both.
//
// Node ESM resolves bare specifiers by walking up from this script's
// directory looking for node_modules/. The Go harness creates a symlink
//   test/parity/node_modules -> ../../sdk/ts/node_modules
// at test setup so imports like "@bufbuild/protobuf" resolve. The
// SDK itself is imported by relative path from sdk/ts/dist/index.js.

import { create } from "@bufbuild/protobuf";
import {
  NervClient,
  EnvelopeSchema,
  DispatchRequestSchema,
  ListRequestSchema,
} from "../../sdk/ts/dist/index.js";

const socket = process.env.NERV_SOCKET;
if (!socket) {
  console.error("NERV_SOCKET unset");
  process.exit(2);
}

const envelope = create(EnvelopeSchema, {
  nervVersion: "1.0.0-draft",
  event: "PreToolUse",
  eventId: "parity-event",
  cli: { name: "claude", version: "0.0.0" },
  session: { id: "parity-session", cwd: "/tmp" },
});

const client = new NervClient({ socket });
try {
  const listRes = await client.events.list(create(ListRequestSchema, {}));
  const firstEventName = listRes.events?.[0]?.name ?? "";
  process.stdout.write(firstEventName + "\n");

  const req = create(DispatchRequestSchema, { envelope });
  const res = await client.events.dispatch(req);
  const message = res.decision?.message ?? "";
  process.stdout.write(message + "\n");
} finally {
  client.close();
}
