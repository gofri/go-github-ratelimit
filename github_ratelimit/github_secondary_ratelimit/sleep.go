package github_secondary_ratelimit

import (
	"context"
	"time"
)

// sleepWithContext sleeps for d duration or until ctx is done.
// Returns nil if the sleep completes successfully, or the error from ctx.
// special thanks to Cedric Bail (@cedric-appdirect) for the original cancellation-aware code.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	select {
	case <-ctx.Done():
		if !timer.Stop() {
			<-timer.C
		}
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
