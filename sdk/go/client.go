package nervsdk

import (
	"context"
	"net/http"

	"connectrpc.com/connect"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	"hop.top/nerv/gen/go/nerv/v1/nervv1connect"
)

// Client dispatches hook envelopes to a running nerv daemon over a
// Unix domain socket. It is safe for concurrent use; the underlying
// http.Client and connect client wrap a stdlib *http.Transport which
// pools connections internally.
type Client struct {
	httpClient *http.Client
	rpc        nervv1connect.EventServiceClient
}

// NewUDS constructs a Client that talks to a nerv daemon listening on
// sockPath. The dial is lazy — the socket is contacted on the first
// RPC, not by this constructor.
func NewUDS(sockPath string) *Client {
	return newClient(udsHTTPClient(sockPath))
}

// NewWithHTTPClient constructs a Client over an arbitrary *http.Client.
// Useful for tests (httptest.Server), TCP transports, or custom dialers.
// baseURL must be the Connect base URL of the daemon (e.g.
// "http://unix" for UDS or "http://127.0.0.1:9000" for TCP).
func NewWithHTTPClient(httpClient *http.Client, baseURL string) *Client {
	c := &Client{httpClient: httpClient}
	c.rpc = nervv1connect.NewEventServiceClient(httpClient, baseURL)
	return c
}

func newClient(httpClient *http.Client) *Client {
	return &Client{
		httpClient: httpClient,
		// "http://unix" is a sentinel host — the UDS transport ignores
		// it and always dials the configured socket.
		rpc: nervv1connect.NewEventServiceClient(httpClient, "http://unix"),
	}
}

// Dispatch sends envelope to the daemon and returns the merged
// decision. envelope must not be nil.
func (c *Client) Dispatch(ctx context.Context, envelope *nervv1.Envelope) (*nervv1.Decision, error) {
	resp, err := c.rpc.Dispatch(ctx, connect.NewRequest(&nervv1.DispatchRequest{
		Envelope: envelope,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetDecision(), nil
}

// List returns the daemon's catalog of known events.
func (c *Client) List(ctx context.Context) ([]*nervv1.EventDescriptor, error) {
	resp, err := c.rpc.List(ctx, connect.NewRequest(&nervv1.ListRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetEvents(), nil
}

// Close releases idle connections held by the underlying transport.
// Safe to call multiple times; safe to call on a never-used Client.
func (c *Client) Close() error {
	if t, ok := c.httpClient.Transport.(*http.Transport); ok {
		t.CloseIdleConnections()
	}
	return nil
}
