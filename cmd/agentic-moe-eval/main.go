// Command agentic-moe-eval runs opt-in, provider-backed capability evaluations.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if !errors.Is(err, errThreshold) {
			fmt.Fprintln(os.Stderr, "agentic-moe-eval:", err)
		}
		os.Exit(1)
	}
}
