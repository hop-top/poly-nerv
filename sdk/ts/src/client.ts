import { createClient } from "@connectrpc/connect";
import type * as http from "node:http";
import { EventService } from "./gen/nerv/v1/event_pb.js";
import { makeTransport, type TransportConfig } from "./transport.js";

/**
 * NervClient wraps the connect-es transport to dispatch events to
 * a running nerv daemon and to query the event catalog.
 *
 * Mirrors the Go SDK's `nervsdk.Client` surface: construct with a
 * UDS path (`socket`) or HTTP base URL (`baseUrl`), call
 * `client.events.dispatch` / `client.events.list`, then `close()`
 * to release the underlying http.Agent's keep-alive sockets.
 *
 * Example:
 *
 *   const client = new NervClient({ socket: "/tmp/nerv.sock" });
 *   try {
 *     const res = await client.events.dispatch({ envelope });
 *     const cat = await client.events.list({});
 *   } finally {
 *     client.close();
 *   }
 */
export class NervClient {
  readonly events: ReturnType<typeof createClient<typeof EventService>>;
  private readonly agent?: http.Agent;

  constructor(cfg: TransportConfig) {
    const built = makeTransport(cfg);
    this.events = createClient(EventService, built.transport);
    this.agent = built.agent;
  }

  /**
   * Release idle connections held by the underlying transport.
   *
   * For UDS clients this destroys the `http.Agent` keep-alive pool;
   * for `baseUrl` clients this is a no-op (connect-node's default
   * fetch transport owns no agent). Safe to call multiple times.
   *
   * Mirrors the Go SDK's `(*Client).Close()` idempotent contract.
   */
  close(): void {
    if (this.agent) {
      this.agent.destroy();
    }
  }
}
