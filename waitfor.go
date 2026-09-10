// Package waitfor checks resources concurrently with configurable retry delays,
// attempt limits, and a shared elapsed-time limit. Context cancellation is
// authoritative; resources must honor their context and return promptly.
//
// Construct a Runner with ResourceConfig values, or register factories through
// Runner.Resources before the first Check, Test, or Run freezes the registry.
// Check returns ordered results and inspectable errors; Test returns only the
// error. Run waits for resources before executing a command with the caller's
// context. Retry options apply only to the waiting phase.
package waitfor

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

type (
	// Program describes a command and the resources it requires before starting.
	Program struct {
		Executable string
		Args       []string
		Resources  []string
	}

	// Runner waits for resources and optionally executes programs. A Runner created
	// with New supports concurrent calls. Its registry freezes on the first valid
	// Check, Test, or Run, even when the resource list is empty.
	Runner struct {
		registry *Registry
	}

	// Result describes one requested resource. Attempts counts Resource.Test calls;
	// resolution failures have zero attempts. Duration includes resolution, attempts,
	// delays, and observer calls. Err retains the final resource error, joined with
	// cancellation errors when applicable. Successful results have a nil Err.
	Result struct {
		Resource string
		Attempts int
		Duration time.Duration
		Err      error
	}

	// Attempt describes a failed attempt for which another retry is planned.
	// Number starts at one; Next is the scheduled delay, including jitter.
	Attempt struct {
		Resource string
		Number   int
		Err      error
		Next     time.Duration
	}

	// Observer defines a callback function triggered on each retry attempt with details of the current attempt.
	Observer func(Attempt)
)

// New creates a Runner. Invalid or duplicate resource configurations return an
// error and a nil Runner. New with no configurations creates an empty registry.
func New(configs ...ResourceConfig) (*Runner, error) {
	registry, err := newRegistry(configs)
	if err != nil {
		return nil, err
	}

	return &Runner{registry: registry}, nil
}

// Resources returns the registry for setup and inspection. Registration is
// allowed only before the first valid Check, Test, or Run.
func (r *Runner) Resources() *Registry {
	return r.registry
}

// Run waits for the program's resources, then executes it with the caller's
// context and returns combined stdout/stderr and the standard os/exec error.
// WithMaxElapsedTime limits the wait, not the command's execution time.
func (r *Runner) Run(ctx context.Context, program Program, setters ...Option) ([]byte, error) {
	if err := r.Test(ctx, program.Resources, setters...); err != nil {
		return nil, err
	}

	return exec.CommandContext(ctx, program.Executable, program.Args...).CombinedOutput()
}

// Test is the error-only convenience form of Check.
func (r *Runner) Test(ctx context.Context, resources []string, setters ...Option) error {
	_, err := r.Check(ctx, resources, setters...)

	return err
}

// Check waits for all resources concurrently and returns results in input order,
// including duplicate inputs. Resource failures return a *WaitError alongside
// the results; they do not cancel other resources. An empty resource list succeeds.
// Invalid options return nil results and ErrInvalidOption before any resource
// work or registry freezing. Cancellation is cooperative: factories and observers
// must return promptly, and Resource.Test must honor its context.
func (r *Runner) Check(ctx context.Context, resources []string, setters ...Option) ([]Result, error) {
	opts, err := newOptions(setters)
	if err != nil {
		return nil, err
	}

	factories := r.registry.freeze()
	if opts.maxElapsedTime > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.maxElapsedTime)
		defer cancel()
	}

	if observer := opts.observer; observer != nil {
		var mu sync.Mutex
		opts.observer = func(attempt Attempt) {
			mu.Lock()
			defer mu.Unlock()

			// Another resource's callback may have consumed the remaining budget.
			if ctx.Err() == nil && retryFits(ctx, attempt.Next) {
				observer(attempt)
			}
		}
	}

	results := make([]Result, len(resources))
	var wg sync.WaitGroup
	wg.Add(len(resources))

	for i, resource := range resources {
		go func() {
			defer wg.Done()
			results[i] = checkResource(ctx, resource, factories, opts)
		}()
	}

	wg.Wait()

	var failures []ResourceError
	for _, result := range results {
		if result.Err != nil {
			failures = append(failures, ResourceError{Resource: result.Resource, Err: result.Err})
		}
	}

	if len(failures) > 0 {
		return results, &WaitError{Errors: failures}
	}

	return results, nil
}
