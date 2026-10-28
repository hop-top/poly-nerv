package main

import (
	"flag"
	"fmt"
	"net/http"
	_ "net/http/pprof"
)

var pprofAddr = flag.String("pprof", "", "enable pprof on `addr` (e.g. localhost:6060); daemon mode only")

// startPprof starts the pprof HTTP server if --pprof is set.
// Called from daemon mode only; stateless stdin->stdout mode ignores it.
//
// Future: wire into daemon loop when nervd gains long-running mode.
func startPprof() {
	if pprofAddr == nil || *pprofAddr == "" {
		return
	}
	go func() {
		_, _ = fmt.Fprintf(flag.CommandLine.Output(), "nervd: pprof listening on %s\n", *pprofAddr)
		_ = http.ListenAndServe(*pprofAddr, nil)
	}()
}
