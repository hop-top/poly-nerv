// Command dispatch is a tiny runnable example that connects to a
// running nerv daemon over its Unix domain socket and prints the
// decision returned for a synthetic PreToolUse envelope.
//
// Usage:
//
//	NERV_SOCKET=/run/user/1000/nerv.sock go run hop.top/nerv/sdk/go/examples/dispatch
//
// or, with the daemon at the default path:
//
//	go run hop.top/nerv/sdk/go/examples/dispatch
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	nervv1 "hop.top/nerv/gen/go/nerv/v1"
	nervsdk "hop.top/nerv/sdk/go"
)

func main() {
	sock := nervsdk.DefaultSocketPath()
	if _, err := os.Stat(sock); err != nil {
		log.Fatalf("nerv daemon socket %s not reachable: %v", sock, err)
	}

	client := nervsdk.NewUDS(sock)
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("close: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	envelope := &nervv1.Envelope{
		NervVersion: "1.0.0-draft",
		Event:       "PreToolUse",
		EventId:     "00000000-0000-0000-0000-000000000001",
		Timestamp:   timestamppb.Now(),
		Cli: &nervv1.CLIInfo{
			Name:    "example-dispatch",
			Version: "0.0.0",
		},
	}

	decision, err := client.Dispatch(ctx, envelope)
	if err != nil {
		log.Fatalf("dispatch: %v", err)
	}

	fmt.Printf("action:  %s\n", decision.GetAction())
	fmt.Printf("message: %s\n", decision.GetMessage())
}
