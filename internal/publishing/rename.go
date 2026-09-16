package publishing

import (
	"context"
	"fmt"
	"time"
)

type renameOperation func(source, destination string) error

func renameWithRetry(
	ctx context.Context,
	source, destination string,
	retryDelays []time.Duration,
	rename renameOperation,
	retryable func(error) bool,
) error {
	err := rename(source, destination)
	for _, delay := range retryDelays {
		if err == nil || !retryable(err) {
			return err
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return fmt.Errorf("%w while retrying directory rename after %v", ctx.Err(), err)
		case <-timer.C:
		}
		err = rename(source, destination)
	}
	return err
}
