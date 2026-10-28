package daemon

import (
	"context"
	"errors"
	"fmt"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	"hop.top/nerv/internal/dispatch"
	"hop.top/nerv/internal/events"
)

// Handler is the production EventService implementation. It routes
// Dispatch requests through the existing internal/dispatch.Dispatcher
// (matcher → handlers → merger → log) and exposes the static event
// catalog via List.
//
// Constructed once at daemon boot; safe for concurrent use because the
// underlying Dispatcher is stateless per-call.
type Handler struct {
	dispatcher *dispatch.Dispatcher
}

// Compile-time guarantee: *Handler implements EventService. If this
// breaks, the server.go Config.Service contract has drifted.
var _ EventService = (*Handler)(nil)

// NewHandler wires a Dispatcher into a Handler. A nil dispatcher is
// rejected: the daemon must have a working dispatch pipeline before
// serving.
func NewHandler(d *dispatch.Dispatcher) (*Handler, error) {
	if d == nil {
		return nil, errors.New("daemon: dispatcher required")
	}
	return &Handler{dispatcher: d}, nil
}

// Dispatch routes a proto DispatchRequest through the internal
// dispatch pipeline and returns the merged decision as proto.
//
// Pipeline parity with nervd stdin mode: same dispatcher, same
// matcher, same merger semantics — only the transport differs.
func (h *Handler) Dispatch(
	_ context.Context, req *nervv1.DispatchRequest,
) (*nervv1.DispatchResponse, error) {
	if req == nil {
		return nil, errors.New("daemon: nil DispatchRequest")
	}

	env, err := protoToEnvelope(req.GetEnvelope())
	if err != nil {
		return nil, fmt.Errorf("daemon: convert envelope: %w", err)
	}

	res, err := h.dispatcher.Dispatch(env)
	if err != nil {
		return nil, fmt.Errorf("daemon: dispatch: %w", err)
	}

	decision, err := decisionToProto(res.Decision)
	if err != nil {
		return nil, fmt.Errorf("daemon: convert decision: %w", err)
	}

	return &nervv1.DispatchResponse{Decision: decision}, nil
}

// List returns the static event catalog as proto EventDescriptors.
// Order matches events.AllEvents() — stable across calls because the
// generator emits events in spec/events.yaml order.
func (h *Handler) List(
	_ context.Context, _ *nervv1.ListRequest,
) (*nervv1.ListResponse, error) {
	all := events.AllEvents()
	out := make([]*nervv1.EventDescriptor, len(all))
	for i, e := range all {
		out[i] = eventToProto(e)
	}
	return &nervv1.ListResponse{Events: out}, nil
}
