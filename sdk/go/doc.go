// Package nervsdk is the Go client SDK for the nerv daemon.
//
// It wraps the generated ConnectRPC EventService client with a Unix
// domain socket transport so downstream Go applications can dispatch
// hook envelopes programmatically against a running nerv daemon.
//
// # Quickstart
//
//	import (
//	    "context"
//	    nervsdk "hop.top/nerv/sdk/go"
//	    nervv1 "hop.top/nerv/gen/go/nerv/v1"
//	)
//
//	client := nervsdk.NewUDS(nervsdk.DefaultSocketPath())
//	defer client.Close()
//
//	decision, err := client.Dispatch(ctx, &nervv1.Envelope{
//	    NervVersion: "1.0.0-draft",
//	    Event:       "PreToolUse",
//	    EventId:     "00000000-0000-0000-0000-000000000000",
//	})
//
// # Module layout
//
// The SDK ships as its own Go module (hop.top/nerv/sdk/go) separate
// from the daemon module (hop.top/nerv). It requires hop.top/nerv for
// the generated protobuf stubs, so the root module's requirements join
// the consumer's module graph; only the SDK, the stubs,
// connectrpc.com/connect and google.golang.org/protobuf are built.
// The module is published as a nested module on the hop-top/nerv
// mirror, tagged sdk/go/vX.Y.Z.
//
//	go get hop.top/nerv/sdk/go
//	import nervsdk "hop.top/nerv/sdk/go"
package nervsdk
