package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/itsmingjie/inference-discovery/reference/go/internal/cli"
	"github.com/itsmingjie/inference-discovery/reference/go/internal/descriptor"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() { <-ctx.Done(); os.Stdin.Close() }()
	if err := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		if ctx.Err() != nil {
			return
		}
		fmt.Fprintln(os.Stderr, "Error:", descriptor.Safe(err.Error()))
		os.Exit(1)
	}
}
