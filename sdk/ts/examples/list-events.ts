/**
 * Minimal example: query the running nerv daemon for its event
 * catalog and print one line per known event.
 *
 * Run:
 *   tsx examples/list-events.ts                 # uses defaultSocketPath()
 *   tsx examples/list-events.ts /tmp/nerv.sock  # explicit socket path
 */
import { NervClient, defaultSocketPath } from "../src/index.js";

const socket = process.argv[2] ?? defaultSocketPath();
const client = new NervClient({ socket });

try {
  const res = await client.events.list({});
  for (const ev of res.events) {
    const flags = [
      ev.blocking ? "blocking" : "non-blocking",
      ev.origin || "unknown",
      ev.category || "uncategorized",
    ].join(" / ");
    console.log(`${ev.name.padEnd(28)} ${flags}`);
  }
} finally {
  client.close();
}
