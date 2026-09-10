package waitfor

import (
	"context"
	"math"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsDefaultsAndOverrides(t *testing.T) {
	opts, err := newOptions(nil)
	require.NoError(t, err)
	assert.Equal(t, options{
		interval: 5 * time.Second, maxInterval: 60 * time.Second,
		attempts: 5, multiplier: 1.5, randomizationFactor: 0.5,
	}, opts)
	opts, err = newOptions([]Option{
		WithInterval(-1), WithInterval(250 * time.Millisecond),
		WithMaxInterval(5 * time.Second), WithAttempts(0),
		WithMaxElapsedTime(30 * time.Second), WithMultiplier(1),
		WithRandomizationFactor(1),
		WithAttemptObserver(func(Attempt) { t.Error("replaced observer called") }),
		WithAttemptObserver(nil),
	})
	require.NoError(t, err)
	assert.Equal(t, 250*time.Millisecond, opts.interval)
	assert.Equal(t, 5*time.Second, opts.maxInterval)
	assert.Equal(t, 30*time.Second, opts.maxElapsedTime)
	assert.Zero(t, opts.attempts)
	assert.Equal(t, float64(1), opts.multiplier)
	assert.Equal(t, float64(1), opts.randomizationFactor)
	assert.Nil(t, opts.observer)
	// A maximum duration is representable when jitter is disabled.
	_, err = newOptions([]Option{WithMaxInterval(math.MaxInt64), WithRandomizationFactor(0)})
	require.NoError(t, err)
}

func TestInvalidOptionsDoNoWork(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{"nil option", []Option{nil}},
		{"zero interval", []Option{WithInterval(0)}},
		{"negative interval", []Option{WithInterval(-time.Second)}},
		{"zero maximum", []Option{WithMaxInterval(0)}},
		{"negative maximum", []Option{WithMaxInterval(-time.Second)}},
		{"maximum below initial", []Option{WithInterval(time.Second), WithMaxInterval(time.Millisecond)}},
		{"negative multiplier", []Option{WithMultiplier(-1)}},
		{"zero multiplier", []Option{WithMultiplier(0)}},
		{"shrinking multiplier", []Option{WithMultiplier(0.5)}},
		{"nan multiplier", []Option{WithMultiplier(math.NaN())}},
		{"infinite multiplier", []Option{WithMultiplier(math.Inf(1))}},
		{"negative infinity multiplier", []Option{WithMultiplier(math.Inf(-1))}},
		{"negative jitter", []Option{WithRandomizationFactor(-0.1)}},
		{"excessive jitter", []Option{WithRandomizationFactor(1.1)}},
		{"nan jitter", []Option{WithRandomizationFactor(math.NaN())}},
		{"infinite jitter", []Option{WithRandomizationFactor(math.Inf(1))}},
		{"negative elapsed", []Option{WithMaxElapsedTime(-time.Second)}},
		{"unrepresentable attempts", []Option{WithAttempts(math.MaxUint64)}},
		{"jitter overflow", []Option{WithMaxInterval(math.MaxInt64)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory := func(*url.URL) (Resource, error) {
				t.Error("invalid options invoked a factory")
				return ResourceFunc(func(context.Context) error {
					t.Error("invalid options invoked a resource")
					return nil
				}), nil
			}
			runner := testRunner(t, factory)
			opts := append([]Option{WithAttemptObserver(func(Attempt) { t.Error("invalid options invoked observer") })}, tc.opts...)
			results, err := runner.Check(context.Background(), []string{"test://a"}, opts...)
			assert.Nil(t, results)
			require.ErrorIs(t, err, ErrInvalidOption)
			assert.NotErrorIs(t, err, ErrWait)
			require.ErrorIs(t, runner.Test(context.Background(), nil, opts...), ErrInvalidOption)
			out, err := runner.Run(context.Background(), Program{}, opts...)
			assert.Nil(t, out)
			require.ErrorIs(t, err, ErrInvalidOption)
			require.NoError(t, runner.Resources().Register("extra", factory), "invalid options must not freeze registration")
		})
	}
}
