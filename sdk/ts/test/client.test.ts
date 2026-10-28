import { describe, it, expect, beforeAll, afterAll, beforeEach } from "vitest";
import { ConnectError, Code } from "@connectrpc/connect";
import { connectNodeAdapter } from "@connectrpc/connect-node";
import { create } from "@bufbuild/protobuf";
import * as http from "node:http";
import * as os from "node:os";
import * as path from "node:path";
import * as fs from "node:fs";
import {
  NervClient,
  EnvelopeSchema,
  DispatchRequestSchema,
  Action,
  defaultSocketPath,
  ENV_SOCKET,
} from "../src/index.js";
import { EventService } from "../src/gen/nerv/v1/event_pb.js";

interface FakeState {
  dispatches: number;
  lists: number;
  /** When set, Dispatch responds with this error instead of the OK decision. */
  dispatchError?: ConnectError;
}

/**
 * Spin up an in-process Connect HTTP/1.1 server bound to a UDS. The
 * SDK's UDS transport uses the same wire protocol (HTTP/1.1 + binary
 * connect framing) as the real daemon's h2c handler in HTTP/1.1
 * downgrade mode.
 */
function startFakeDaemon(
  state: FakeState,
): Promise<{ socketPath: string; close: () => Promise<void> }> {
  return new Promise((resolve, reject) => {
    // macOS sun_path limit is 104 bytes; keep the dir short.
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "nerv-ts-"));
    const socketPath = path.join(tmpDir, "n.sock");

    const handler = connectNodeAdapter({
      routes(router) {
        router.service(EventService, {
          dispatch(req) {
            state.dispatches += 1;
            if (state.dispatchError) {
              throw state.dispatchError;
            }
            const env = req.envelope;
            const msg = env?.event ? `received:${env.event}` : "ok";
            return {
              decision: {
                action: Action.ALLOW,
                message: msg,
              },
            };
          },
          list() {
            state.lists += 1;
            return {
              events: [
                {
                  name: "PreToolUse",
                  category: "tool",
                  origin: "native",
                  blocking: true,
                },
                {
                  name: "SessionStart",
                  category: "session",
                  origin: "native",
                  blocking: false,
                },
              ],
            };
          },
        });
      },
    });

    const server = http.createServer(handler);
    server.on("error", reject);
    server.listen(socketPath, () => {
      resolve({
        socketPath,
        close: () =>
          new Promise<void>((res) => {
            server.close(() => {
              try {
                fs.rmSync(tmpDir, { recursive: true, force: true });
              } catch {
                // ignore cleanup races
              }
              res();
            });
          }),
      });
    });
  });
}

describe("NervClient construction", () => {
  it("constructs against a socket without throwing", () => {
    const client = new NervClient({ socket: "/tmp/nerv-nonexistent.sock" });
    expect(client.events).toBeDefined();
    expect(typeof client.events.dispatch).toBe("function");
    expect(typeof client.events.list).toBe("function");
    client.close();
  });

  it("rejects empty config", () => {
    expect(() => new NervClient({} as never)).toThrow(/socket or baseUrl/);
  });

  it("constructs against an HTTP baseUrl", () => {
    const client = new NervClient({ baseUrl: "http://127.0.0.1:7777" });
    expect(client.events).toBeDefined();
    client.close(); // no-op on baseUrl path; must not throw
  });
});

describe("NervClient.dispatch", () => {
  let server: { socketPath: string; close: () => Promise<void> };
  let state: FakeState;

  beforeAll(async () => {
    state = { dispatches: 0, lists: 0 };
    server = await startFakeDaemon(state);
  });

  beforeEach(() => {
    state.dispatches = 0;
    state.lists = 0;
    state.dispatchError = undefined;
  });

  afterAll(async () => {
    await server.close();
  });

  it("round-trips a dispatch over UDS", async () => {
    const client = new NervClient({ socket: server.socketPath });
    try {
      const envelope = create(EnvelopeSchema, {
        nervVersion: "1.0.0-draft",
        event: "PreToolUse",
        eventId: "00000000-0000-0000-0000-000000000000",
      });
      const req = create(DispatchRequestSchema, { envelope });
      const res = await client.events.dispatch(req);
      expect(res.decision).toBeDefined();
      expect(res.decision?.action).toBe(Action.ALLOW);
      expect(res.decision?.message).toBe("received:PreToolUse");
      expect(state.dispatches).toBe(1);
    } finally {
      client.close();
    }
  });

  it("propagates ConnectError from the server", async () => {
    state.dispatchError = new ConnectError(
      "denied",
      Code.PermissionDenied,
    );
    const client = new NervClient({ socket: server.socketPath });
    try {
      const envelope = create(EnvelopeSchema, {
        nervVersion: "1.0.0-draft",
        event: "PreToolUse",
        eventId: "11111111-1111-1111-1111-111111111111",
      });
      const req = create(DispatchRequestSchema, { envelope });
      await expect(client.events.dispatch(req)).rejects.toBeInstanceOf(
        ConnectError,
      );
      try {
        await client.events.dispatch(req);
      } catch (err) {
        expect(err).toBeInstanceOf(ConnectError);
        expect((err as ConnectError).code).toBe(Code.PermissionDenied);
      }
    } finally {
      client.close();
    }
  });
});

describe("NervClient.list", () => {
  let server: { socketPath: string; close: () => Promise<void> };
  let state: FakeState;

  beforeAll(async () => {
    state = { dispatches: 0, lists: 0 };
    server = await startFakeDaemon(state);
  });

  afterAll(async () => {
    await server.close();
  });

  it("round-trips a list over UDS", async () => {
    const client = new NervClient({ socket: server.socketPath });
    try {
      const res = await client.events.list({});
      expect(res.events).toHaveLength(2);
      expect(res.events[0].name).toBe("PreToolUse");
      expect(res.events[1].name).toBe("SessionStart");
    } finally {
      client.close();
    }
  });
});

describe("NervClient.close", () => {
  it("is idempotent on a UDS client", () => {
    const client = new NervClient({ socket: "/tmp/nerv-nonexistent.sock" });
    expect(() => {
      client.close();
      client.close();
    }).not.toThrow();
  });

  it("is a no-op on a baseUrl client", () => {
    const client = new NervClient({ baseUrl: "http://127.0.0.1:7777" });
    expect(() => client.close()).not.toThrow();
  });
});

describe("defaultSocketPath", () => {
  const originals: Record<string, string | undefined> = {};

  beforeEach(() => {
    originals.NERV_SOCKET = process.env[ENV_SOCKET];
    originals.XDG_RUNTIME_DIR = process.env.XDG_RUNTIME_DIR;
    delete process.env[ENV_SOCKET];
    delete process.env.XDG_RUNTIME_DIR;
  });

  afterAll(() => {
    if (originals.NERV_SOCKET === undefined) {
      delete process.env[ENV_SOCKET];
    } else {
      process.env[ENV_SOCKET] = originals.NERV_SOCKET;
    }
    if (originals.XDG_RUNTIME_DIR === undefined) {
      delete process.env.XDG_RUNTIME_DIR;
    } else {
      process.env.XDG_RUNTIME_DIR = originals.XDG_RUNTIME_DIR;
    }
  });

  it("respects NERV_SOCKET override", () => {
    process.env[ENV_SOCKET] = "/custom/path.sock";
    expect(defaultSocketPath()).toBe("/custom/path.sock");
  });

  it("uses XDG_RUNTIME_DIR when set", () => {
    process.env.XDG_RUNTIME_DIR = "/run/user/1000";
    expect(defaultSocketPath()).toBe("/run/user/1000/nerv.sock");
  });

  it("falls back to tmpdir when XDG_RUNTIME_DIR unset", () => {
    const expected = path.join(os.tmpdir(), "nerv.sock");
    expect(defaultSocketPath()).toBe(expected);
  });

  it("treats empty NERV_SOCKET as unset", () => {
    process.env[ENV_SOCKET] = "";
    process.env.XDG_RUNTIME_DIR = "/run/user/1000";
    expect(defaultSocketPath()).toBe("/run/user/1000/nerv.sock");
  });
});
