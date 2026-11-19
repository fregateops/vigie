package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// After the first signal, restore default handling so a second Ctrl-C
	// kills a run whose cleanup hangs.
	go func() { <-ctx.Done(); stop() }()
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(3) // user error
	}
}
