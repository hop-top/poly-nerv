package daemon

import (
	"context"

	"connectrpc.com/connect"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	"hop.top/nerv/gen/go/nerv/v1/nervv1connect"
)

// connectEventService adapts a plain EventService into a ConnectRPC
// handler. It only exists so the EventService interface (used by tests
// and business logic) stays free of connect.Request wrappers.
type connectEventService struct {
	inner EventService
}

var _ nervv1connect.EventServiceHandler = (*connectEventService)(nil)

func (c *connectEventService) Dispatch(
	ctx context.Context, req *connect.Request[nervv1.DispatchRequest],
) (*connect.Response[nervv1.DispatchResponse], error) {
	resp, err := c.inner.Dispatch(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (c *connectEventService) List(
	ctx context.Context, req *connect.Request[nervv1.ListRequest],
) (*connect.Response[nervv1.ListResponse], error) {
	resp, err := c.inner.List(ctx, req.Msg)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}
