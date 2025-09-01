package waitfor

import (
	"time"
)

type (
	// options contains configuration parameters for resource testing behavior.
	// These options control retry intervals, maximum wait times, and the number
	// of attempts made when testing resource availability.
	options struct {
		interval            time.Duration // Initial retry interval between attempts
		maxInterval         time.Duration // Maximum interval for exponential backoff
		attempts            uint64        // Maximum number of retry attempts
		multiplier          float64       // Multiplier for exponential backoff
		randomizationFactor float64       // Randomization factor for backoff intervals
	}

	// Option is a function type used to configure options through the functional
	// options pattern. This allows flexible and extensible configuration of
	// resource testing behavior.
	Option func(opts *options)
)

// newOptions creates a new options instance with default values and applies
// the provided option setters. Default values are:
// - interval: 5 seconds
// - maxInterval: 60 seconds
// - attempts: 5.
// - multiplier: 1.5
// - randomizationFactor: 0.5
func newOptions(setters []Option) *options {
	opts := &options{
		interval:            time.Duration(5) * time.Second,
		maxInterval:         time.Duration(60) * time.Second,
		attempts:            5,
		multiplier:          1.5,
		randomizationFactor: 0.5,
	}

	for _, setter := range setters {
		setter(opts)
	}

	return opts
}

// WithInterval creates an Option that sets the initial retry interval in seconds.
// This interval is used as the starting point for exponential backoff between
// retry attempts. The actual interval will increase exponentially up to maxInterval.
//
// Example:
//
//	runner.Test(ctx, resources, waitfor.WithInterval(2)) // Start with 2 second intervals
func WithInterval(interval uint64) Option {
	return func(opts *options) {
		opts.interval = time.Duration(interval) * time.Second
	}
}

// WithMaxInterval creates an Option that sets the maximum retry interval in seconds.
// When using exponential backoff, the retry interval will not exceed this value.
// This prevents excessively long waits between retry attempts.
//
// Example:
//
//	runner.Test(ctx, resources, waitfor.WithMaxInterval(30)) // Cap at 30 seconds
func WithMaxInterval(interval uint64) Option {
	return func(opts *options) {
		opts.maxInterval = time.Duration(interval) * time.Second
	}
}

// WithAttempts creates an Option that sets the maximum number of retry attempts.
// If a resource test fails this many times, the resource is considered unavailable.
// Set to 0 for unlimited attempts (not recommended without context timeout).
//
// Example:
//
//	runner.Test(ctx, resources, waitfor.WithAttempts(10)) // Try up to 10 times
func WithAttempts(attempts uint64) Option {
	return func(opts *options) {
		opts.attempts = attempts
	}
}

// WithMultiplier creates an Option that sets the multiplier for exponential backoff.
// This value determines how quickly the retry interval increases after each attempt.
// A higher multiplier results in faster growth of the interval.
//
// Example:
//
//	runner.Test(ctx, resources, waitfor.WithMultiplier(2.0)) // Double the interval each time
func WithMultiplier(multiplier float64) Option {
	return func(opts *options) {
		opts.multiplier = multiplier
	}
}

// WithRandomizationFactor creates an Option that sets the randomization factor for
// exponential backoff. This factor introduces jitter to the retry intervals,
// helping to prevent thundering herd problems when multiple clients are retrying
// simultaneously.
//
// Example:
//
//	runner.Test(ctx, resources, waitfor.WithRandomizationFactor(0.5)) // 50% jitter
func WithRandomizationFactor(factor float64) Option {
	return func(opts *options) {
		opts.randomizationFactor = factor
	}
}
