package waitfor

import (
	"errors"
	"fmt"
	"math"
	"time"

	goopt "github.com/ziflex/go-options"
)

type (
	options struct {
		interval            time.Duration
		maxInterval         time.Duration
		maxElapsedTime      time.Duration
		attempts            uint64
		multiplier          float64
		randomizationFactor float64
		observer            Observer
	}

	// Option configures a wait. Each supplied value is validated when applied,
	// even if a later option replaces it. Valid setters apply in order, with the
	// last winning. Nil options are ignored. Relationships between final values
	// are checked before Check, Test, or Run performs any resource work.
	Option = goopt.Option[options]
)

var validateOptionRelationships = goopt.Check(func(o options) error {
	var errs []error

	if o.maxInterval < o.interval {
		errs = append(errs, goopt.ValidationError{
			Field:  "max interval",
			Value:  o.maxInterval.String(),
			Reason: errors.New("must be at least interval"),
		})
	}

	if o.randomizationFactor > 0 && float64(o.maxInterval)*(1+o.randomizationFactor)+1 >= float64(math.MaxInt64) {
		// The delay generator randomizes before we cap the actual delay. Reject
		// values that could overflow its float-to-duration conversion first.
		errs = append(errs, goopt.ValidationError{
			Field:  "max interval",
			Value:  o.maxInterval.String(),
			Reason: errors.New("jitter calculation overflows time.Duration"),
		})
	}

	return errors.Join(errs...)
})

func newOptions(setters []Option) (options, error) {
	opts := options{
		interval:            5 * time.Second,
		maxInterval:         60 * time.Second,
		attempts:            5,
		multiplier:          1.5,
		randomizationFactor: 0.5,
	}

	opts, err := goopt.ApplyTo(opts, setters...)
	if err == nil {
		err = validateOptionRelationships(opts)
	}

	if err != nil {
		return options{}, fmt.Errorf("%w: %w", ErrInvalidOption, err)
	}

	return opts, nil
}

// WithInterval sets the initial retry interval. It must be positive; the default
// is 5 seconds. Durations are used directly, e.g. 500*time.Millisecond.
func WithInterval(interval time.Duration) Option {
	return goopt.New(func(opts *options, input time.Duration) { opts.interval = input }).
		Value(interval).
		Named("interval").
		Validators(goopt.Positive[time.Duration]()).
		Build()
}

// WithMaxInterval caps the actual retry delay, including jitter. It must be
// positive and at least the initial interval; the default is 60 seconds.
func WithMaxInterval(interval time.Duration) Option {
	return goopt.New(func(opts *options, input time.Duration) { opts.maxInterval = input }).
		Value(interval).
		Named("max interval").
		Validators(goopt.Positive[time.Duration]()).
		Build()
}

// WithAttempts limits the total Resource.Test calls per resource, including the
// first call. The default is 5. Zero means unlimited attempts, subject only to
// success, caller cancellation, or an explicitly configured elapsed-time limit.
// A nonzero limit must fit in an int, as used by Result.Attempts.
func WithAttempts(attempts uint64) Option {
	return goopt.New(func(opts *options, input uint64) { opts.attempts = input }).
		Value(attempts).
		Named("attempts").
		Validators(goopt.Max(uint64(^uint(0) >> 1))).
		Build()
}

// WithMaxElapsedTime sets one deadline shared by all resources in a wait. Zero
// (the default) disables this limit. Negative values are invalid. The earlier
// caller deadline wins. Expiration cancels context-aware attempts and is
// inspectable as context.DeadlineExceeded. This option does not limit a Program.
func WithMaxElapsedTime(limit time.Duration) Option {
	return goopt.New(func(opts *options, input time.Duration) { opts.maxElapsedTime = input }).
		Value(limit).
		Named("max elapsed time").
		Validators(goopt.NonNegative[time.Duration]()).
		Build()
}

// WithMultiplier sets exponential growth between retries. It must be finite and
// at least 1; 1 gives a constant base interval. The default is 1.5.
func WithMultiplier(multiplier float64) Option {
	return goopt.New(func(opts *options, input float64) { opts.multiplier = input }).
		Value(multiplier).
		Named("multiplier").
		Validators(goopt.Between[float64](1, math.MaxFloat64)).
		Build()
}

// WithRandomizationFactor sets jitter in the range [0, 1]. The default is 0.5;
// zero disables jitter. Actual delays remain capped by WithMaxInterval. Timing
// values whose jitter calculation could overflow time.Duration are invalid.
func WithRandomizationFactor(factor float64) Option {
	return goopt.New(func(opts *options, input float64) { opts.randomizationFactor = input }).
		Value(factor).
		Named("randomization factor").
		Validators(goopt.Between[float64](0, 1)).
		Build()
}

// WithAttemptObserver observes failed attempts when another retry is planned.
// Calls are synchronous and serialized within each Check, Test, or Run. The
// callback must return promptly; its duration counts toward the wait deadline.
// It cannot change the retry decision or delay through its arguments. A planned
// retry may still be canceled before execution. Callers sharing callback state
// across separate concurrent waits must synchronize that state. Nil disables it.
func WithAttemptObserver(fn func(Attempt)) Option {
	return goopt.New(func(opts *options, input func(Attempt)) { opts.observer = input }).
		Value(fn).
		Named("attempt observer").
		Build()
}
