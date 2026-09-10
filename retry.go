package waitfor

import (
	"context"
	"errors"
	"time"

	"github.com/cenkalti/backoff/v5"
)

func checkResource(ctx context.Context, location string, factories map[string]ResourceFactory, opts options) (result Result) {
	started := time.Now()
	result.Resource = location
	defer func() { result.Duration = time.Since(started) }()

	if result.Err = cancellationError(ctx, nil); result.Err != nil {
		return result
	}

	resource, err := resolveResource(location, func(scheme string) ResourceFactory { return factories[scheme] })
	result.Err = err

	if err := cancellationError(ctx, result.Err); err != nil {
		result.Err = err

		return result
	}

	if result.Err != nil {
		return result
	}

	delays := backoff.NewExponentialBackOff()
	delays.InitialInterval = opts.interval
	delays.MaxInterval = opts.maxInterval
	delays.Multiplier = opts.multiplier
	delays.RandomizationFactor = opts.randomizationFactor
	delays.Reset()

	for {
		if err := cancellationError(ctx, result.Err); err != nil {
			result.Err = err

			return result
		}

		result.Attempts++
		err := resource.Test(ctx)
		if err != nil {
			result.Err = err
		}

		if err := cancellationError(ctx, result.Err); err != nil {
			result.Err = err
			return result
		}

		if err == nil {
			result.Err = nil
			return result
		}

		if opts.attempts > 0 && uint64(result.Attempts) >= opts.attempts {
			return result
		}

		next := min(delays.NextBackOff(), opts.maxInterval)

		if opts.observer != nil && retryFits(ctx, next) {
			opts.observer(Attempt{Resource: location, Number: result.Attempts, Err: err, Next: next})
		}

		waitForRetry(ctx, next)
	}
}

func retryFits(ctx context.Context, delay time.Duration) bool {
	deadline, ok := ctx.Deadline()

	return !ok || time.Until(deadline) > delay
}

func waitForRetry(ctx context.Context, delay time.Duration) {
	if !retryFits(ctx, delay) {
		// Expire at the actual deadline, without announcing an impossible retry.
		<-ctx.Done()

		return
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func cancellationError(ctx context.Context, last error) error {
	err := ctx.Err()
	if err == nil {
		return nil
	}

	cause := context.Cause(ctx)
	if !errors.Is(cause, err) {
		return errors.Join(last, err, cause)
	}

	return errors.Join(last, err)
}
