package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"example.com/ol-tech35/pulse/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
