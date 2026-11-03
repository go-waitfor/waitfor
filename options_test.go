package waitfor

import (
	"context"
	"errors"
	"math"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	goopt "github.com/ziflex/go-options"
)

func TestOptionsDefaultsAndOverrides(t *testing.T) {
	opts, err := newOptions(nil)
	require.NoError(t, err)
	assert.Equal(t, options{
		interval: 5 * time.Second, maxInterval: 60 * time.Second,
		attempts: 5, multiplier: 1.5, randomizationFactor: 0.5,
	}, opts)
	opts, err = newOptions([]Option{
		WithInterval(time.Second), WithInterval(250 * time.Millisecond),
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

	opts, err = newOptions([]Option{
		WithMaxElapsedTime(time.Second), WithMaxElapsedTime(0),
		WithRandomizationFactor(0), WithAttempts(0),
	})
	require.NoError(t, err)
	assert.Zero(t, opts.maxElapsedTime)
	assert.Zero(t, opts.randomizationFactor)
	assert.Zero(t, opts.attempts)
}

func TestNilOptionsAreIgnored(t *testing.T) {
	defaults, err := newOptions(nil)
	require.NoError(t, err)
	opts, err := newOptions([]Option{nil, nil})
	require.NoError(t, err)
	assert.Equal(t, defaults, opts)

	opts, err = newOptions([]Option{nil, WithInterval(time.Second), nil, WithInterval(2 * time.Second), nil})
	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, opts.interval)

	runner := testRunner(t, readyFactory)
	results, err := runner.Check(testContext(t), []string{"test://a"}, nil, WithAttempts(1), nil)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 1, results[0].Attempts)
	assert.NoError(t, results[0].Err)
}

func TestOptionsNumericBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{"minimum intervals", []Option{WithInterval(time.Nanosecond), WithMaxInterval(time.Nanosecond)}},
		{"maximum duration without jitter", []Option{WithMaxInterval(math.MaxInt64), WithRandomizationFactor(0)}},
		{"maximum attempts", []Option{WithAttempts(uint64(^uint(0) >> 1))}},
		{"maximum elapsed time", []Option{WithMaxElapsedTime(math.MaxInt64)}},
		{"maximum finite multiplier", []Option{WithMultiplier(math.MaxFloat64)}},
		{"maximum jitter below overflow", []Option{WithMaxInterval(math.MaxInt64/2 - 1024), WithRandomizationFactor(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newOptions(tc.opts)
			require.NoError(t, err)
		})
	}
}

func TestOptionRelationshipsUseFinalValues(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{"initial above default maximum", []Option{WithInterval(2 * time.Minute), WithMaxInterval(3 * time.Minute)}},
		{"maximum below default initial", []Option{WithMaxInterval(2 * time.Second), WithInterval(time.Second)}},
		{"maximum duration without jitter", []Option{WithMaxInterval(math.MaxInt64), WithRandomizationFactor(0)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			forward, err := newOptions(tc.opts)
			require.NoError(t, err)
			reverse, err := newOptions([]Option{tc.opts[1], tc.opts[0]})
			require.NoError(t, err)
			assert.Equal(t, forward, reverse)
		})
	}

	_, err := newOptions([]Option{
		WithInterval(2 * time.Minute), WithInterval(time.Second),
		WithMaxInterval(math.MaxInt64), WithMaxInterval(time.Minute),
	})
	require.NoError(t, err)
}

func TestOptionValidationErrors(t *testing.T) {
	t.Run("option failures are joined", func(t *testing.T) {
		cause := errors.New("custom option failure")
		opts, err := newOptions([]Option{
			WithInterval(0), WithInterval(time.Second),
			func(*options) error { return cause },
			WithMultiplier(math.NaN()),
		})
		assert.Equal(t, options{}, opts)
		require.ErrorIs(t, err, ErrInvalidOption)
		require.ErrorIs(t, err, cause)
		var validationErr goopt.ValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Equal(t, "interval", validationErr.Field)
		assert.NotEmpty(t, validationErr.Value)
		assert.ErrorContains(t, err, "multiplier")
	})

	t.Run("relationship failures are joined", func(t *testing.T) {
		opts, err := newOptions([]Option{
			WithInterval(math.MaxInt64),
			WithMaxInterval(math.MaxInt64 - 1),
			WithRandomizationFactor(1),
		})
		assert.Equal(t, options{}, opts)
		require.ErrorIs(t, err, ErrInvalidOption)
		var validationErr goopt.ValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Equal(t, "max interval", validationErr.Field)
		assert.NotEmpty(t, validationErr.Value)
		assert.ErrorContains(t, err, "must be at least interval")
		assert.ErrorContains(t, err, "overflows time.Duration")
	})

	t.Run("relationships are skipped after option failures", func(t *testing.T) {
		_, err := newOptions([]Option{WithInterval(0), WithMaxInterval(math.MaxInt64)})
		require.ErrorIs(t, err, ErrInvalidOption)
		var validationErr goopt.ValidationError
		require.ErrorAs(t, err, &validationErr)
		assert.Equal(t, "interval", validationErr.Field)
		assert.NotContains(t, err.Error(), "overflows time.Duration")
	})
}

func TestInvalidOptionsDoNoWork(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []Option
	}{
		{"zero interval", []Option{WithInterval(0)}},
		{"negative interval", []Option{WithInterval(-time.Second)}},
		{"replaced invalid interval", []Option{WithInterval(-1), WithInterval(time.Second)}},
		{"invalid option among nils", []Option{nil, WithInterval(0), nil}},
		{"zero maximum", []Option{WithMaxInterval(0)}},
		{"negative maximum", []Option{WithMaxInterval(-time.Second)}},
		{"maximum below initial", []Option{WithInterval(time.Second), WithMaxInterval(time.Millisecond)}},
		{"negative multiplier", []Option{WithMultiplier(-1)}},
		{"zero multiplier", []Option{WithMultiplier(0)}},
		{"shrinking multiplier", []Option{WithMultiplier(0.5)}},
		{"multiplier just below one", []Option{WithMultiplier(math.Nextafter(1, 0))}},
		{"nan multiplier", []Option{WithMultiplier(math.NaN())}},
		{"infinite multiplier", []Option{WithMultiplier(math.Inf(1))}},
		{"negative infinity multiplier", []Option{WithMultiplier(math.Inf(-1))}},
		{"negative jitter", []Option{WithRandomizationFactor(-0.1)}},
		{"excessive jitter", []Option{WithRandomizationFactor(1.1)}},
		{"jitter just above one", []Option{WithRandomizationFactor(math.Nextafter(1, math.Inf(1)))}},
		{"jitter just below zero", []Option{WithRandomizationFactor(-math.SmallestNonzeroFloat64)}},
		{"nan jitter", []Option{WithRandomizationFactor(math.NaN())}},
		{"infinite jitter", []Option{WithRandomizationFactor(math.Inf(1))}},
		{"negative infinite jitter", []Option{WithRandomizationFactor(math.Inf(-1))}},
		{"negative elapsed", []Option{WithMaxElapsedTime(-time.Second)}},
		{"unrepresentable attempts", []Option{WithAttempts(math.MaxUint64)}},
		{"attempts above platform maximum", []Option{WithAttempts(uint64(^uint(0)>>1) + 1)}},
		{"jitter overflow", []Option{WithMaxInterval(math.MaxInt64)}},
		{"maximum jitter above overflow", []Option{WithMaxInterval(math.MaxInt64/2 + 1024), WithRandomizationFactor(1)}},
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
			var validationErr goopt.ValidationError
			require.ErrorAs(t, err, &validationErr)
			assert.NotEmpty(t, validationErr.Field)
			require.ErrorIs(t, runner.Test(context.Background(), nil, opts...), ErrInvalidOption)
			out, err := runner.Run(context.Background(), Program{}, opts...)
			assert.Nil(t, out)
			require.ErrorIs(t, err, ErrInvalidOption)
			require.NoError(t, runner.Resources().Register("extra", factory), "invalid options must not freeze registration")
		})
	}
}
