package cli

import (
	"context"
	"os"
	"os/signal"
)

// WorkContext cancels queued work on Ctrl+C. Active codec calls finish before
// returning so they cannot leave half-published outputs.
func WorkContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt)
}
