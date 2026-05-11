package nightshift

import (
	"context"
	"fmt"
	"time"

	nsv1 "github.com/nightshiftco/nightshift/gen/go/nightshift/v1"
)

// IsTerminal reports whether the run has reached a terminal status:
// COMPLETED, ERROR, or INTERRUPTED.
func IsTerminal(s nsv1.RunStatus) bool {
	switch s {
	case nsv1.RunStatus_RUN_STATUS_COMPLETED,
		nsv1.RunStatus_RUN_STATUS_ERROR,
		nsv1.RunStatus_RUN_STATUS_INTERRUPTED:
		return true
	}
	return false
}

// WaitForTerminal polls GetRun with exponential backoff (base → maxInterval,
// doubling each tick) until the run reaches a terminal status or the
// context expires. The caller controls the hard wall via ctx deadline.
func (c *Client) WaitForTerminal(ctx context.Context, runID string, base, maxInterval time.Duration) (nsv1.RunStatus, error) {
	if base <= 0 {
		base = 2 * time.Second
	}
	if maxInterval < base {
		maxInterval = base
	}
	interval := base
	for {
		run, err := c.GetRun(ctx, runID)
		if err != nil {
			return nsv1.RunStatus_RUN_STATUS_UNSPECIFIED, fmt.Errorf("getrun: %w", err)
		}
		if IsTerminal(run.GetStatus()) {
			return run.GetStatus(), nil
		}
		select {
		case <-ctx.Done():
			return run.GetStatus(), ctx.Err()
		case <-time.After(interval):
		}
		if interval < maxInterval {
			interval *= 2
			if interval > maxInterval {
				interval = maxInterval
			}
		}
	}
}
