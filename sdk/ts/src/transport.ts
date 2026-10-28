import { createConnectTransport } from "@connectrpc/connect-node";
import type { Transport } from "@connectrpc/connect";
import * as http from "node:http";
import * as os from "node:os";
import * as path from "node:path";

/**
 * Env var consulted by {@link defaultSocketPath} to override the
 * XDG-derived default. Mirrors the Go SDK's `EnvSocket` constant and
 * the nerv daemon convention (cmd/nervd, `nerv daemon`).
 */
export const ENV_SOCKET = "NERV_SOCKET";

/**
 * Resolve the conventional UDS path for a local nerv daemon:
 * `$NERV_SOCKET` if set, otherwise `$XDG_RUNTIME_DIR/nerv.sock`,
 * falling back to `$TMPDIR/nerv.sock` when `XDG_RUNTIME_DIR` is unset.
 *
 * Mirrors `nervsdk.DefaultSocketPath()` in the Go SDK.
 */
export function defaultSocketPath(): string {
  const override = process.env[ENV_SOCKET];
  if (override && override.length > 0) {
    return override;
  }
  const runtimeDir = process.env.XDG_RUNTIME_DIR ?? os.tmpdir();
  return path.join(runtimeDir, "nerv.sock");
}

/**
 * TransportConfig selects how the SDK reaches the nerv daemon.
 *
 * Exactly one of `socket` or `baseUrl` must be set.
 *  - `socket`: absolute path to the nerv UDS (production default).
 *  - `baseUrl`: HTTP(S) endpoint (cross-machine, testing).
 *
 * h2c is intentionally NOT used: the daemon's h2c listener also speaks
 * HTTP/1.1, and HTTP/1.1-over-UDS sidesteps the connect-node h2c agent
 * limitations.
 *
 * The Go SDK exposes a `NewWithHTTPClient(*http.Client, baseURL)`
 * constructor for arbitrary transports. The TS analogue is the
 * `baseUrl` form here — connect-es transports are the universal
 * handle in the Node ecosystem, so we don't expose a separate
 * "bring-your-own-transport" escape hatch.
 */
export interface TransportConfig {
  /** Unix socket path. Mutually exclusive with baseUrl. */
  socket?: string;
  /** HTTP(S) base URL. Required for cross-machine. */
  baseUrl?: string;
}

/**
 * Built transport plus the optional resources the SDK owns so the
 * client can release them on close. `agent` is set only for the UDS
 * path; baseUrl callers manage their own connection lifecycle.
 */
export interface BuiltTransport {
  transport: Transport;
  agent?: http.Agent;
}

export function makeTransport(cfg: TransportConfig): BuiltTransport {
  if (cfg.socket) {
    const agent = new http.Agent({
      // socketPath is supported at runtime but absent from upstream types
      // @ts-expect-error — node http.AgentOptions omits socketPath
      socketPath: cfg.socket,
    });
    const transport = createConnectTransport({
      baseUrl: "http://unix",
      httpVersion: "1.1",
      nodeOptions: { agent } as Parameters<
        typeof createConnectTransport
      >[0] extends { httpVersion: "1.1"; nodeOptions?: infer O }
        ? O
        : never,
    });
    return { transport, agent };
  }
  if (!cfg.baseUrl) {
    throw new Error("nerv: socket or baseUrl required");
  }
  return {
    transport: createConnectTransport({
      baseUrl: cfg.baseUrl,
      httpVersion: "1.1",
    }),
  };
}
