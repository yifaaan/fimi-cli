package runtime

import (
	"context"
	"time"

	"fimi-cli/internal/contextstore"
	runtimeevents "fimi-cli/internal/runtime/events"
)

const retryBackoffBaseDelay = 200 * time.Millisecond
const retryBackoffMaxDelay = 2 * time.Second
const retryBackoffJitterWindow = 100 * time.Millisecond

func (r Runner) runStepWithRetry(
	ctx context.Context,
	store contextstore.Context,
	cfg StepConfig,
	runStep func(ctx context.Context, store contextstore.Context, cfg StepConfig) (StepResult, error),
) (StepResult, error) {
	var lastErr error
	retryStatusEmitted := false
	maxAttempts := 1 + r.config.MaxAdditionalRetriesPerStep
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return StepResult{}, ctx.Err()
		}

		stepResult, err := runStep(ctx, store, cfg)
		if err == nil {
			return stepResult, nil
		}
		if !IsRetryable(err) {
			return StepResult{}, err
		}

		lastErr = err
		if attempt == maxAttempts {
			break
		}

		delay := r.retryBackoffDelay(attempt)
		if emitErr := r.emitRetryStatusUpdate(ctx, store, &runtimeevents.RetryStatus{
			Attempt:     attempt,
			MaxAttempts: maxAttempts,
			NextDelayMS: delay.Milliseconds(),
		}); emitErr != nil {
			return StepResult{}, emitErr
		}
		retryStatusEmitted = true
		if sleepErr := r.retrySleep(ctx, delay); sleepErr != nil {
			return StepResult{}, sleepErr
		}
	}

	if retryStatusEmitted {
		if clearErr := r.emitRetryStatusUpdate(ctx, store, nil); clearErr != nil {
			return StepResult{}, clearErr
		}
	}

	return StepResult{}, lastErr
}

func (r Runner) retryBackoffDelay(attempt int) time.Duration {
	if r.retryBackoffDelayFn != nil {
		return r.retryBackoffDelayFn(attempt)
	}

	return calculateRetryBackoffDelay(attempt, retryBackoffJitter())
}

func calculateRetryBackoffDelay(attempt int, jitter time.Duration) time.Duration {
	baseDelay := retryBackoffBaseDelayForAttempt(attempt)
	clampedJitter := clampRetryBackoffJitter(jitter)
	return clampRetryBackoffDelay(baseDelay + clampedJitter)
}

func retryBackoffBaseDelayForAttempt(attempt int) time.Duration {
	if attempt <= 1 {
		return retryBackoffBaseDelay
	}

	baseDelay := retryBackoffBaseDelay
	for i := 1; i < attempt; i++ {
		if baseDelay >= retryBackoffMaxDelay/2 {
			return retryBackoffMaxDelay
		}
		baseDelay *= 2
	}

	return clampRetryBackoffDelay(baseDelay)
}

func retryBackoffJitter() time.Duration {
	window := retryBackoffJitterWindow
	if window <= 0 {
		return 0
	}

	return time.Duration(time.Now().UnixNano() % int64(window+1))
}

func clampRetryBackoffJitter(jitter time.Duration) time.Duration {
	if jitter <= 0 {
		return 0
	}
	if jitter > retryBackoffJitterWindow {
		return retryBackoffJitterWindow
	}
	return jitter
}

func clampRetryBackoffDelay(delay time.Duration) time.Duration {
	if delay <= 0 {
		return retryBackoffBaseDelay
	}
	if delay > retryBackoffMaxDelay {
		return retryBackoffMaxDelay
	}
	return delay
}

func (r Runner) retrySleep(ctx context.Context, delay time.Duration) error {
	if r.retrySleepFn != nil {
		return r.retrySleepFn(ctx, delay)
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r Runner) shieldContextWrite(ctx context.Context, write func() error) error {
	if r.shieldContextWriteFn != nil {
		return r.shieldContextWriteFn(ctx, write)
	}
	if err := write(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	return nil
}
