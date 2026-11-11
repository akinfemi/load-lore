package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// untilInterrupted returns a context that the first Ctrl-C or SIGTERM
// cancels, for commands that run several designs in a row. runner.Run
// catches the same signal to tear its run down, but only cancels its own
// context, so without this the command would go on to the next run.
//
// After the first signal it stops catching them, so a second Ctrl-C during
// teardown still exits at once, as it does for a single run.
func untilInterrupted(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()
	return ctx, stop
}
