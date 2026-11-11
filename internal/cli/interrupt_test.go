package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

// One Ctrl-C must end both the run in progress (runner.Run's own handler)
// and the command's sequence of runs.
func TestUntilInterruptedSharesTheSignalWithTheRun(t *testing.T) {
	seq, stopSeq := untilInterrupted(context.Background())
	defer stopSeq()
	run, stopRun := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopRun()

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	for name, ctx := range map[string]context.Context{"sequence": seq, "run": run} {
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
			t.Fatalf("the %s context wasn't canceled by SIGINT", name)
		}
	}
}
