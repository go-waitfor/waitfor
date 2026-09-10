package waitfor

import (
	"fmt"
	"math"
	"time"
)

type options struct {
	interval            time.Duration
	maxInterval         time.Duration
	maxElapsedTime      time.Duration
	attempts            uint64
	multiplier          float64
	randomizationFactor float64
	observer            Observer
}

// Option configures a wait. Final values are validated together by Check, Test,
// or Run before any resource work. When repeated, the last setter wins.
type Option func(opts *options)

func newOptions(setters []Option) (options, error) {
	opts := options{
		interval:            5 * time.Second,
		maxInterval:         60 * time.Second,
		attempts:            5,
		multiplier:          1.5,
		randomizationFactor: 0.5,
	}

	for _, setter := range setters {
		if setter == nil {
			return options{}, fmt.Errorf("%w: nil option", ErrInvalidOption)
		}
		setter(&opts)
	}

	if err := opts.validate(); err != nil {
		return options{}, err
	}

	return opts, nil
}

func (o options) validate() error {
	switch {
	case o.interval <= 0:
		return fmt.Errorf("%w: interval must be positive", ErrInvalidOption)
	case o.maxInterval <= 0:
		return fmt.Errorf("%w: max interval must be positive", ErrInvalidOption)
	case o.maxInterval < o.interval:
		return fmt.Errorf("%w: max interval must be at least interval", ErrInvalidOption)
	case math.IsNaN(o.multiplier) || math.IsInf(o.multiplier, 0) || o.multiplier < 1:
		return fmt.Errorf("%w: multiplier must be finite and at least 1", ErrInvalidOption)
	case math.IsNaN(o.randomizationFactor) || math.IsInf(o.randomizationFactor, 0) || o.randomizationFactor < 0 || o.randomizationFactor > 1:
		return fmt.Errorf("%w: randomization factor must be finite and between 0 and 1", ErrInvalidOption)
	case o.maxElapsedTime < 0:
		return fmt.Errorf("%w: max elapsed time must not be negative", ErrInvalidOption)
	case o.attempts > uint64(^uint(0)>>1):
		return fmt.Errorf("%w: attempts must fit in an int", ErrInvalidOption)
	case o.randomizationFactor > 0 && float64(o.maxInterval)*(1+o.randomizationFactor)+1 >= float64(math.MaxInt64):
		// The delay generator randomizes before we cap the actual delay. Reject
		// values that could overflow its float-to-duration conversion first.
		return fmt.Errorf("%w: max interval with jitter overflows time.Duration", ErrInvalidOption)
	}

	return nil
}

// WithInterval sets the initial retry interval. It must be positive; the default
// is 5 seconds. Durations are used directly, e.g. 500*time.Millisecond.
func WithInterval(interval time.Duration) Option {
	return func(opts *options) { opts.interval = interval }
}

// WithMaxInterval caps the actual retry delay, including jitter. It must be
// positive and at least the initial interval; the default is 60 seconds.
func WithMaxInterval(interval time.Duration) Option {
	return func(opts *options) { opts.maxInterval = interval }
}

// WithAttempts limits the total Resource.Test calls per resource, including the
// first call. The default is 5. Zero means unlimited attempts, subject only to
// success, caller cancellation, or an explicitly configured elapsed-time limit.
// A nonzero limit must fit in an int, as used by Result.Attempts.
func WithAttempts(attempts uint64) Option {
	return func(opts *options) { opts.attempts = attempts }
}

// WithMaxElapsedTime sets one deadline shared by all resources in a wait. Zero
// (the default) disables this limit. Negative values are invalid. The earlier
// caller deadline wins. Expiration cancels context-aware attempts and is
// inspectable as context.DeadlineExceeded. This option does not limit a Program.
func WithMaxElapsedTime(limit time.Duration) Option {
	return func(opts *options) { opts.maxElapsedTime = limit }
}

// WithMultiplier sets exponential growth between retries. It must be finite and
// at least 1; 1 gives a constant base interval. The default is 1.5.
func WithMultiplier(multiplier float64) Option {
	return func(opts *options) { opts.multiplier = multiplier }
}

// WithRandomizationFactor sets jitter in the range [0, 1]. The default is 0.5;
// zero disables jitter. Actual delays remain capped by WithMaxInterval. Timing
// values whose jitter calculation could overflow time.Duration are invalid.
func WithRandomizationFactor(factor float64) Option {
	return func(opts *options) { opts.randomizationFactor = factor }
}

// WithAttemptObserver observes failed attempts when another retry is planned.
// Calls are synchronous and serialized within each Check, Test, or Run. The
// callback must return promptly; its duration counts toward the wait deadline.
// It cannot change the retry decision or delay through its arguments. A planned
// retry may still be canceled before execution. Callers sharing callback state
// across separate concurrent waits must synchronize that state. Nil disables it.
func WithAttemptObserver(fn func(Attempt)) Option {
	return func(opts *options) { opts.observer = fn }
}
