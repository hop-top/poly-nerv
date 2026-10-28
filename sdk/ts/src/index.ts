// Daemon client surface (Go SDK parity surface)
export { NervClient } from "./client.js";
export {
  defaultSocketPath,
  ENV_SOCKET,
  type TransportConfig,
} from "./transport.js";
export * from "./gen/nerv/v1/event_pb.js";

// Handler primitives (prose-track scaffold; unchanged)
export {
  createHandler,
  type Handler,
  type Decision as HandlerDecision,
  type Envelope as HandlerEnvelope,
} from "./handler.js";
export { events, type Event } from "./events.js";
