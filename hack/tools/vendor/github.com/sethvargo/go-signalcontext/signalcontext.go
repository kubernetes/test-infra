// Package signalcontext creates context.Contexts that cancel on os.Signals.
//
//     ctx, cancel := signalcontext.OnInterrupt()
//     defer cancel()
//
package signalcontext

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// OnInterrupt creates a new context that cancels on SIGINT or SIGTERM.
func OnInterrupt() (context.Context, func()) {
	return Wrap(context.Background(), syscall.SIGINT, syscall.SIGTERM)
}

// On creates a new context that cancels on the given signals.
func On(signals ...os.Signal) (context.Context, func()) {
	return Wrap(context.Background(), signals...)
}

// Wrap creates a new context that cancels on the given signals. It wraps the
// provided context.
func Wrap(ctx context.Context, signals ...os.Signal) (context.Context, func()) {
	ctx, closer := context.WithCancel(ctx)

	c := make(chan os.Signal, 1)
	signal.Notify(c, signals...)

	go func() {
		select {
		case <-c:
			closer()
		case <-ctx.Done():
		}
	}()

	return ctx, closer
}
