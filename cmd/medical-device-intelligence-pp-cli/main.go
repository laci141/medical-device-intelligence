// Command medical-device-intelligence-pp-cli is the entry point. It is a thin
// shell: version handling plus delegation to internal/cli.Dispatch, which owns
// the command surface. No cobra — command parsing stays on the standard library.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/laci141/medical-device-intelligence/internal/cli"
)

const version = "0.1.0-phase2b"

func main() {
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "version", "--version", "-v":
			fmt.Println("medical-device-intelligence-pp-cli", version)
			return
		}
	}
	// Ctrl+C cancels the context so long-running commands (watch) stop
	// cleanly. No global deadline: the HTTP layer already bounds each request
	// (30s timeout + one retry), and watch is legitimately long-lived.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(cli.Dispatch(ctx, os.Stdout, os.Stderr, os.Args[1:]))
}
