package waitfor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cenkalti/backoff/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testRunner(t *testing.T, factory ResourceFactory) *Runner {
	t.Helper()
	runner, err := New(ResourceConfig{Scheme: []string{"test"}, Factory: factory})
	require.NoError(t, err)
	return runner
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func fastOptions(extra ...Option) []Option {
	return append([]Option{
		WithInterval(time.Millisecond),
		WithMaxInterval(4 * time.Millisecond),
		WithMultiplier(1),
		WithRandomizationFactor(0),
	}, extra...)
}

type readinessError struct{ message string }

func (e *readinessError) Error() string { return e.message }

func TestCheckResults(t *testing.T) {
	failure := &readinessError{"not ready"}
	wrapped := fmt.Errorf("probe: %w", failure)
	for _, tc := range []struct {
		name      string
		resources []string
		failures  []string
	}{
		{"empty", nil, nil},
		{"success", []string{"test://a", "test://b"}, nil},
		{"mixed", []string{"test://a", "test://bad", "test://b"}, []string{"test://bad"}},
		{"all fail", []string{"test://bad", "test://bad"}, []string{"test://bad", "test://bad"}},
		{"duplicate success", []string{"test://a", "test://a"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var factories atomic.Int32
			runner := testRunner(t, func(u *url.URL) (Resource, error) {
				factories.Add(1)
				return ResourceFunc(func(context.Context) error {
					if u.Host == "bad" {
						return wrapped
					}
					return nil
				}), nil
			})
			results, err := runner.Check(testContext(t), tc.resources, WithAttempts(1))
			require.Len(t, results, len(tc.resources))
			require.EqualValues(t, len(tc.resources), factories.Load())
			for i, result := range results {
				assert.Equal(t, tc.resources[i], result.Resource)
				assert.Equal(t, 1, result.Attempts)
				assert.Greater(t, result.Duration, time.Duration(0))
				if result.Resource == "test://bad" {
					assert.Same(t, wrapped, result.Err)
				} else {
					assert.NoError(t, result.Err)
				}
			}
			if len(tc.failures) == 0 {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrWait)
			require.ErrorIs(t, err, wrapped)
			require.ErrorIs(t, err, failure)
			var aggregate *WaitError
			require.ErrorAs(t, err, &aggregate)
			var typed *readinessError
			require.ErrorAs(t, err, &typed)
			assert.Same(t, failure, typed)
			var resourceError ResourceError
			require.ErrorAs(t, err, &resourceError)
			assert.Equal(t, tc.failures[0], resourceError.Resource)
			require.Len(t, aggregate.Errors, len(tc.failures))
			for i, resource := range tc.failures {
				assert.Equal(t, resource, aggregate.Errors[i].Resource)
				assert.Same(t, wrapped, aggregate.Errors[i].Err)
			}
			require.ErrorIs(t, runner.Test(testContext(t), tc.resources, WithAttempts(1)), failure)
		})
	}
}

func TestCheckConcurrentOrderedFailures(t *testing.T) {
	ctx := testContext(t)
	first, second, third := make(chan struct{}), make(chan struct{}), make(chan struct{})
	entered := make(chan string, 3)
	sentinels := map[string]error{"first": errors.New("one"), "second": errors.New("two"), "third": errors.New("three")}
	gates := map[string]chan struct{}{"first": first, "second": second, "third": third}
	runner := testRunner(t, func(u *url.URL) (Resource, error) {
		return ResourceFunc(func(ctx context.Context) error {
			entered <- u.Host
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-gates[u.Host]:
			}
			// Arrange resource completions in reverse input order.
			switch u.Host {
			case "third":
				close(second)
			case "second":
				close(first)
			}
			return sentinels[u.Host]
		}), nil
	})
	type outcome struct {
		results []Result
		err     error
	}
	done := make(chan outcome, 1)
	go func() {
		results, err := runner.Check(ctx, []string{"test://first", "test://second", "test://third"}, WithAttempts(1))
		done <- outcome{results, err}
	}()
	for range 3 {
		select {
		case <-entered:
		case <-ctx.Done():
			t.Fatal("resources did not start concurrently")
		}
	}
	close(third)
	var got outcome
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("wait did not finish")
	}
	require.EqualError(t, got.err, ErrWait.Error()+": test://first: one; test://second: two; test://third: three")
	for i, name := range []string{"first", "second", "third"} {
		assert.Equal(t, "test://"+name, got.results[i].Resource)
		assert.Same(t, sentinels[name], got.results[i].Err)
		require.ErrorIs(t, got.err, sentinels[name])
	}
}

func TestCheckResolutionErrors(t *testing.T) {
	failure := fmt.Errorf("factory: %w", errors.New("configuration"))
	runner := testRunner(t, func(u *url.URL) (Resource, error) {
		if u.Host == "nil" {
			return nil, nil
		}
		return nil, failure
	})
	results, err := runner.Check(testContext(t), []string{"test://bad", "unknown://bad", "://bad", "test://nil"},
		WithAttemptObserver(func(Attempt) { t.Error("resolution error was retried") }))
	require.ErrorIs(t, err, ErrWait)
	require.ErrorIs(t, err, failure)
	require.ErrorIs(t, err, ErrResourceNotFound)
	require.ErrorIs(t, err, ErrInvalidArgument)
	var parseError *url.Error
	require.ErrorAs(t, err, &parseError)
	for _, result := range results {
		assert.Zero(t, result.Attempts)
		assert.Error(t, result.Err)
	}
}

func TestCheckRetriesAndDelays(t *testing.T) {
	failure := errors.New("not ready")
	for _, succeed := range []bool{false, true} {
		t.Run(fmt.Sprint(succeed), func(t *testing.T) {
			calls := 0
			var events []Attempt
			runner := testRunner(t, func(*url.URL) (Resource, error) {
				return ResourceFunc(func(context.Context) error {
					calls++
					if succeed && calls == 5 {
						return nil
					}
					return failure
				}), nil
			})
			results, err := runner.Check(testContext(t), []string{"test://a"}, fastOptions(
				WithAttempts(5), WithMultiplier(2),
				WithAttemptObserver(func(a Attempt) { events = append(events, a) }),
			)...)
			require.Len(t, events, 4)
			for i, delay := range []time.Duration{time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond, 4 * time.Millisecond} {
				assert.Equal(t, Attempt{Resource: "test://a", Number: i + 1, Err: failure, Next: delay}, events[i])
			}
			assert.Equal(t, 5, calls)
			assert.Equal(t, 5, results[0].Attempts)
			assert.GreaterOrEqual(t, results[0].Duration, 11*time.Millisecond)
			if succeed {
				require.NoError(t, err)
				assert.NoError(t, results[0].Err)
			} else {
				require.ErrorIs(t, err, failure)
				assert.Same(t, failure, results[0].Err)
			}
		})
	}
}

func TestCheckJitterAndActualDelayCap(t *testing.T) {
	failure := errors.New("not ready")
	var events []Attempt
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error { return failure }), nil
	})
	results, err := runner.Check(testContext(t), []string{"test://a"},
		WithAttempts(40), WithInterval(time.Millisecond), WithMaxInterval(time.Millisecond),
		WithRandomizationFactor(0.5),
		WithAttemptObserver(func(a Attempt) { events = append(events, a) }))
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 40, results[0].Attempts)
	require.Len(t, events, 39)
	for _, event := range events {
		assert.GreaterOrEqual(t, event.Next, 500*time.Microsecond)
		assert.LessOrEqual(t, event.Next, time.Millisecond)
	}
}

func TestCheckUnlimitedHasNoImplicitElapsedLimit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watchdog := time.AfterFunc(5*time.Second, cancel)
	defer watchdog.Stop()
	// A delay beyond the dependency's old default budget must still be planned.
	// Cancel from the observer so this regression takes no actual long wait.
	failure := errors.New("not ready")
	observed := false
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(ctx context.Context) error {
			_, hasDeadline := ctx.Deadline()
			assert.False(t, hasDeadline)
			return failure
		}), nil
	})
	results, err := runner.Check(ctx, []string{"test://a"},
		WithAttempts(0), WithInterval(20*time.Minute), WithMaxInterval(20*time.Minute),
		WithMaxElapsedTime(0), WithRandomizationFactor(0),
		WithAttemptObserver(func(a Attempt) {
			observed = true
			assert.Equal(t, 20*time.Minute, a.Next)
			cancel()
		}))
	require.True(t, observed)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 1, results[0].Attempts)
}

func TestCheckUnlimitedAttempts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watchdog := time.AfterFunc(5*time.Second, cancel)
	defer watchdog.Stop()
	failure := errors.New("not ready")
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error { return failure }), nil
	})
	results, err := runner.Check(ctx, []string{"test://a"}, fastOptions(
		WithAttempts(0), WithAttemptObserver(func(a Attempt) {
			if a.Number == 7 {
				cancel()
			}
		}),
	)...)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 7, results[0].Attempts)
}

func TestCheckElapsedLimits(t *testing.T) {
	failure := errors.New("not ready")
	for _, tc := range []struct {
		name     string
		attempts uint64
		budget   time.Duration
		deadline bool
	}{
		{"elapsed only", 0, 20 * time.Millisecond, true},
		{"elapsed before attempts", 100, 20 * time.Millisecond, true},
		{"attempts before elapsed", 1, time.Second, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := testRunner(t, func(*url.URL) (Resource, error) {
				return ResourceFunc(func(context.Context) error { return failure }), nil
			})
			// No next retry fits the short budget; expiration must still wait
			// for the deadline and retain the first resource failure.
			started := time.Now()
			results, err := runner.Check(testContext(t), []string{"test://a"},
				WithAttempts(tc.attempts), WithInterval(time.Second), WithMaxInterval(time.Second),
				WithRandomizationFactor(0), WithMaxElapsedTime(tc.budget),
				WithAttemptObserver(func(Attempt) { t.Error("unexpected retry notification") }))
			require.ErrorIs(t, err, failure)
			assert.Equal(t, 1, results[0].Attempts)
			assert.Equal(t, tc.deadline, errors.Is(err, context.DeadlineExceeded))
			if tc.deadline {
				assert.GreaterOrEqual(t, time.Since(started), tc.budget)
			} else {
				assert.Same(t, failure, results[0].Err)
			}
		})
	}
}

func TestCheckSharedDeadlineCancelsInProgressAttempts(t *testing.T) {
	failure := errors.New("probe stopped")
	deadlines := make(chan time.Time, 2)
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			deadlines <- deadline
			<-ctx.Done()
			return failure
		}), nil
	})
	results, err := runner.Check(testContext(t), []string{"test://a", "test://b"},
		WithAttempts(1), WithMaxElapsedTime(100*time.Millisecond))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, failure)
	require.Len(t, deadlines, 2)
	assert.Equal(t, <-deadlines, <-deadlines)
	for _, result := range results {
		assert.Equal(t, 1, result.Attempts)
		assert.ErrorIs(t, result.Err, context.DeadlineExceeded)
		assert.ErrorIs(t, result.Err, failure)
	}
}

func TestCheckCallerDeadlineAndCause(t *testing.T) {
	cause, failure := errors.New("caller deadline"), errors.New("probe stopped")
	ctx, cancel := context.WithTimeoutCause(context.Background(), 50*time.Millisecond, cause)
	defer cancel()
	deadline, _ := ctx.Deadline()
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(ctx context.Context) error {
			got, _ := ctx.Deadline()
			assert.Equal(t, deadline, got)
			<-ctx.Done()
			return failure
		}), nil
	})
	results, err := runner.Check(ctx, []string{"test://a"}, WithAttempts(0), WithMaxElapsedTime(time.Hour))
	require.ErrorIs(t, err, ErrWait)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 1, results[0].Attempts)
}

func TestCheckPreCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		t.Error("canceled wait invoked factory")
		return nil, nil
	})
	results, err := runner.Check(ctx, []string{"test://a", "test://b"}, WithAttempts(1))
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, ErrWait)
	for _, result := range results {
		assert.Zero(t, result.Attempts)
		assert.ErrorIs(t, result.Err, context.Canceled)
	}
	empty, err := runner.Check(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestCheckCancellationWinsOverFinalAttempt(t *testing.T) {
	failure, cause := errors.New("probe failure"), errors.New("shutdown")
	for _, returned := range []error{nil, failure} {
		t.Run(fmt.Sprint(returned), func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			runner := testRunner(t, func(*url.URL) (Resource, error) {
				return ResourceFunc(func(context.Context) error {
					cancel(cause)
					return returned
				}), nil
			})
			results, err := runner.Check(ctx, []string{"test://a"}, WithAttempts(1),
				WithAttemptObserver(func(Attempt) { t.Error("canceled attempt was observed") }))
			require.ErrorIs(t, err, context.Canceled)
			require.ErrorIs(t, err, cause)
			if returned != nil {
				require.ErrorIs(t, err, returned)
			}
			assert.Equal(t, 1, results[0].Attempts)
		})
	}
}

func TestCheckCancellationDuringResolution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("factory failed")
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		cancel()
		return nil, failure
	})
	results, err := runner.Check(ctx, []string{"test://a"})
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, failure)
	assert.Zero(t, results[0].Attempts)
}

func TestCheckCancellationDuringDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	failure := errors.New("not ready")
	observed := make(chan struct{})
	done := make(chan error, 1)
	var calls atomic.Int32
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error { calls.Add(1); return failure }), nil
	})
	go func() {
		done <- runner.Test(ctx, []string{"test://a"}, WithAttempts(0),
			WithInterval(time.Hour), WithMaxInterval(time.Hour), WithRandomizationFactor(0),
			WithAttemptObserver(func(Attempt) { close(observed) }))
	}()
	select {
	case <-observed:
	case <-testContext(t).Done():
		t.Fatal("retry was not scheduled")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
		require.ErrorIs(t, err, failure)
	case <-testContext(t).Done():
		t.Fatal("cancellation did not interrupt the delay")
	}
	assert.EqualValues(t, 1, calls.Load())
}

func TestCheckObserverSerializedAndSynchronous(t *testing.T) {
	failure := errors.New("not ready")
	const count = 12
	var events []Attempt // Deliberately no caller lock within this single wait.
	var calls atomic.Int32
	started := make(chan struct{}, count)
	release := make(chan struct{})
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		n := 0
		return ResourceFunc(func(ctx context.Context) error {
			n++
			calls.Add(1)
			if n == 1 {
				started <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
				return failure
			}
			return nil
		}), nil
	})
	resources := make([]string, count)
	for i := range resources {
		resources[i] = fmt.Sprintf("test://resource-%d", i)
	}
	ctx := testContext(t)
	done := make(chan error, 1)
	go func() {
		done <- runner.Test(ctx, resources, fastOptions(WithAttempts(2),
			WithAttemptObserver(func(a Attempt) { events = append(events, a) }))...)
	}()
	for range count {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("resources did not start concurrently")
		}
	}
	close(release)
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("wait did not finish")
	}
	require.Len(t, events, count)
	assert.EqualValues(t, count*2, calls.Load())
	for _, event := range events {
		assert.Equal(t, 1, event.Number)
		assert.Same(t, failure, event.Err)
		assert.Equal(t, time.Millisecond, event.Next)
	}
}

func TestCheckObserverTimeCountsTowardDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	failure := errors.New("not ready")
	var events int
	var observerDuration time.Duration
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error { return failure }), nil
	})
	results, err := runner.Check(ctx, []string{"test://a"}, fastOptions(
		WithAttempts(0), WithAttemptObserver(func(Attempt) {
			events++
			started := time.Now()
			<-ctx.Done()
			observerDuration = time.Since(started)
		}),
	)...)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 1, events)
	assert.Equal(t, 1, results[0].Attempts)
	assert.GreaterOrEqual(t, results[0].Duration, observerDuration)
}

func TestCheckDoesNotInterpretBackoffErrors(t *testing.T) {
	failure := backoff.Permanent(errors.New("not ready"))
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error { return failure }), nil
	})
	results, err := runner.Check(testContext(t), []string{"test://a"}, fastOptions(WithAttempts(2))...)
	require.ErrorIs(t, err, failure)
	assert.Same(t, failure, results[0].Err)
	assert.Equal(t, 2, results[0].Attempts)
}

func TestCheckRedactsSensitiveLocations(t *testing.T) {
	for _, tc := range []struct{ location, label string }{
		{"test://example-user:example-pass@host/db", "test://host/db"},
		{"test://host/db?token=example-token", "test://host/db"},
		{"test://host/db#example-fragment", "test://host/db"},
		{"test:example-opaque", "test:"},
	} {
		failure := fmt.Errorf("probe failed for %s", tc.location)
		runner := testRunner(t, func(*url.URL) (Resource, error) {
			return ResourceFunc(func(context.Context) error { return failure }), nil
		})
		results, err := runner.Check(testContext(t), []string{tc.location}, WithAttempts(1))
		require.ErrorIs(t, err, failure)
		require.Equal(t, tc.location, results[0].Resource)
		require.Same(t, failure, results[0].Err)
		var entry ResourceError
		require.ErrorAs(t, err, &entry)
		require.Equal(t, tc.location, entry.Resource)
		for _, formatted := range []string{entry.Error(), err.Error()} {
			assert.Contains(t, formatted, tc.label)
			assert.NotContains(t, formatted, "example-")
		}
	}
}

func TestMalformedResourceURLErrors(t *testing.T) {
	const location = "test://user:secret-password@host:secret-port/db?token=secret-token"
	runner := testRunner(t, readyFactory)
	_, resolveErr := runner.Resources().Resolve(location)
	results, checkErr := runner.Check(testContext(t), []string{location})
	require.Equal(t, location, results[0].Resource)
	require.Zero(t, results[0].Attempts)
	for _, err := range []error{resolveErr, results[0].Err, checkErr} {
		var parseErr *url.Error
		require.ErrorAs(t, err, &parseErr)
		assert.Equal(t, location, parseErr.URL)
		assert.NotContains(t, err.Error(), "secret-")
	}

	// A missing colon makes this a valid relative path to the URL parser.
	err := runner.Test(testContext(t), []string{"test//user:secret-password@host/db"})
	require.ErrorIs(t, err, ErrResourceNotFound)
	require.NotContains(t, err.Error(), "secret-password")
}

func TestCheckPermanentFailure(t *testing.T) {
	cause := errors.New("access denied")
	failure := fmt.Errorf("probe: %w", Permanent(cause))
	runner := testRunner(t, func(u *url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error {
			if u.Host == "bad" {
				return failure
			}
			return Permanent(nil)
		}), nil
	})
	results, err := runner.Check(testContext(t), []string{"test://bad", "test://ready"}, WithAttempts(0),
		WithAttemptObserver(func(Attempt) { t.Error("permanent failure scheduled a retry") }))
	require.ErrorIs(t, err, cause)
	require.Same(t, failure, results[0].Err)
	require.Equal(t, 1, results[0].Attempts)
	require.NoError(t, results[1].Err)
}

func TestCheckWrappedCancellationCause(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	shutdown, denied := errors.New("service shutting down"), errors.New("access denied")
	cause := errors.Join(context.Canceled, shutdown)
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error {
			cancel(cause)
			return Permanent(denied)
		}), nil
	})
	results, err := runner.Check(ctx, []string{"test://host"})
	for _, target := range []error{context.Canceled, cause, shutdown, denied} {
		require.ErrorIs(t, err, target)
		require.ErrorIs(t, results[0].Err, target)
	}
}

func TestBackoffFractionalNanosecondGrowth(t *testing.T) {
	failure := errors.New("offline")
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error { return failure }), nil
	})
	var delays []time.Duration
	err := runner.Test(testContext(t), []string{"test://host"}, WithInterval(1), WithMaxInterval(7),
		WithMultiplier(1.5), WithRandomizationFactor(0), WithAttempts(8),
		WithAttemptObserver(func(a Attempt) { delays = append(delays, a.Next) }))
	require.ErrorIs(t, err, failure)
	require.Equal(t, []time.Duration{1, 2, 3, 4, 6, 7, 7}, delays)
}

func TestCheckConcurrencyLimit(t *testing.T) {
	for _, stopEarly := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", stopEarly), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				inputs := []string{"test://a", "test://b", "test://a", "test://c"}
				resolved := make(chan string, len(inputs))
				release := make(chan struct{})
				runner := testRunner(t, func(u *url.URL) (Resource, error) {
					resolved <- u.Host
					return ResourceFunc(func(ctx context.Context) error {
						select {
						case <-release:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					}), nil
				})
				var results []Result
				var err error
				go func() { results, err = runner.Check(ctx, inputs, WithMaxConcurrency(2)) }()
				synctest.Wait()
				require.Len(t, resolved, 2)
				if stopEarly {
					cancel()
				} else {
					close(release)
				}
				synctest.Wait()
				require.Len(t, results, len(inputs))
				if stopEarly {
					require.ErrorIs(t, err, context.Canceled)
					require.Len(t, resolved, 2)
				} else {
					require.NoError(t, err)
					require.Len(t, resolved, len(inputs))
				}
				for i, r := range results {
					assert.Equal(t, inputs[i], r.Resource)
					if stopEarly && i >= 2 {
						assert.Zero(t, r.Attempts)
						assert.ErrorIs(t, r.Err, context.Canceled)
					} else {
						assert.Equal(t, 1, r.Attempts)
					}
				}
			})
		})
	}
}

func TestRunnerConcurrentChecks(t *testing.T) {
	runner := testRunner(t, func(*url.URL) (Resource, error) {
		return ResourceFunc(func(context.Context) error { return nil }), nil
	})
	ctx := testContext(t)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, runner.Test(ctx, []string{"test://a", "test://b"}))
			assert.Equal(t, []string{"test"}, runner.Resources().List())
			_, err := runner.Resources().Resolve("test://a")
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}

func TestResourceFunc(t *testing.T) {
	ctx := testContext(t)
	failure := errors.New("probe")
	var resource Resource = ResourceFunc(func(got context.Context) error {
		assert.Same(t, ctx, got)
		return failure
	})
	assert.Same(t, failure, resource.Test(ctx))
}
