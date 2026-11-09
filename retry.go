package waitfor

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
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

	base := opts.interval

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

		if _, permanent := errors.AsType[*permanentError](err); permanent {
			return result
		}

		next := opts.retryDelay(base)

		if opts.observer != nil && retryFits(ctx, next) {
			opts.observer(Attempt{Resource: location, Number: result.Attempts, Err: err, Next: next})
		}

		waitForRetry(ctx, next)
		base = opts.nextInterval(base)
	}
}

func (o options) nextInterval(base time.Duration) time.Duration {
	if o.multiplier == 1 || base == o.maxInterval {
		return base
	}

	scaled := float64(base) * o.multiplier
	if scaled >= float64(o.maxInterval) {
		return o.maxInterval
	}

	// A fractional nanosecond of growth would otherwise round down forever.
	return max(base+1, time.Duration(scaled))
}

func (o options) retryDelay(base time.Duration) time.Duration {
	if o.randomizationFactor == 0 {
		return base
	}

	spread := float64(base) * o.randomizationFactor
	delay := float64(base) - spread + rand.Float64()*(2*spread+1)
	if delay >= float64(o.maxInterval) {
		return o.maxInterval
	}

	return time.Duration(delay)
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
	if errors.Is(cause, err) {
		return errors.Join(last, cause)
	}

	return errors.Join(last, err, cause)
}
