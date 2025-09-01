// Package waitfor provides utilities for testing and waiting for resource availability
// before executing programs. It supports various resource types through a plugin system
// and offers configurable retry mechanisms with exponential backoff.
//
// This package is designed for scenarios where applications need to wait for dependencies
// like databases, web services, files, or other external resources to be available
// before starting up.
//
// Basic usage:
//
//	runner := waitfor.New(
//		postgres.Use(),
//		http.Use(),
//	)
//
//	err := runner.Test(ctx, []string{
//		"postgres://user:pass@localhost:5432/db",
//		"http://localhost:8080/health",
//	})
//
// The package supports custom resource types through the ResourceConfig interface
// and provides flexible configuration options for retry behavior and timeouts.
package waitfor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/cenkalti/backoff"
)

type (
	// Program represents a command to execute along with the resources
	// that must be available before execution. It encapsulates the executable
	// path, command arguments, and dependency resource URLs.
	Program struct {
		Executable string   // The path or name of the executable to run
		Args       []string // Command line arguments for the executable
		Resources  []string // List of resource URLs that must be available
	}

	// Runner is the main component responsible for testing resource availability
	// and executing programs. It maintains a registry of resource types and
	// provides methods to test resources and run programs conditionally.
	Runner struct {
		registry *Registry
	}
)

// New creates a new Runner instance with the specified resource configurations.
// Each ResourceConfig defines how to handle specific resource URL schemes.
// Multiple configurations can be provided to support different resource types.
//
// Example:
//
//	runner := waitfor.New(
//		postgres.Use(),
//		http.Use(),
//		fs.Use(),
//	)
func New(configurators ...ResourceConfig) *Runner {
	r := new(Runner)
	r.registry = newRegistry(configurators)

	return r
}

// Resources returns the resource registry associated with this Runner.
// The registry can be used to manually register additional resource types
// or query available resource schemes.
func (r *Runner) Resources() *Registry {
	return r.registry
}

// Run tests resource availability and executes the given program if all resources are ready.
// It first validates that all resources specified in program.Resources are available,
// then executes the program's command if the tests pass. Returns the combined output
// from the executed command or an error if resources are not ready or execution fails.
//
// The setters parameter allows customization of retry behavior, timeouts, and intervals.
//
// Example:
//
//	program := waitfor.Program{
//		Executable: "myapp",
//		Args:       []string{"--config", "prod.yaml"},
//		Resources:  []string{"postgres://localhost:5432/db", "http://api:8080/health"},
//	}
//	output, err := runner.Run(ctx, program, waitfor.WithAttempts(10))
func (r *Runner) Run(ctx context.Context, program Program, setters ...Option) ([]byte, error) {
	err := r.Test(ctx, program.Resources, setters...)

	if err != nil {
		return nil, err
	}

	cmd := exec.Command(program.Executable, program.Args...)

	return cmd.CombinedOutput()
}

// Test validates that all specified resources are available and responding correctly.
// It tests each resource concurrently using their respective Test implementations
// with configurable retry logic and exponential backoff. Returns an error if any
// resource fails its availability test after all retry attempts are exhausted.
//
// The setters parameter allows customization of retry behavior including:
// - Initial retry interval (WithInterval)
// - Maximum retry interval (WithMaxInterval)  
// - Number of retry attempts (WithAttempts)
//
// Example:
//
//	resources := []string{
//		"postgres://user:pass@localhost:5432/db",
//		"http://localhost:8080/health",
//		"file://./config.json",
//	}
//	err := runner.Test(ctx, resources, waitfor.WithAttempts(5), waitfor.WithInterval(2))
func (r *Runner) Test(ctx context.Context, resources []string, setters ...Option) error {
	opts := newOptions(setters)

	var buff bytes.Buffer
	output := r.testAllInternal(ctx, resources, *opts)

	for err := range output {
		if err != nil {
			buff.WriteString(err.Error() + ";")
		}
	}

	if buff.Len() != 0 {
		return fmt.Errorf("%s: %s", ErrWait, buff.String())
	}

	return nil
}

// testAllInternal concurrently tests all provided resources and returns a channel
// of errors. Each resource is tested in its own goroutine with the specified options.
// The channel is closed when all tests complete.
func (r *Runner) testAllInternal(ctx context.Context, resources []string, opts Options) <-chan error {
	var wg sync.WaitGroup
	wg.Add(len(resources))

	output := make(chan error, len(resources))

	for _, resource := range resources {
		resource := resource

		go func() {
			defer wg.Done()

			output <- r.testInternal(ctx, resource, opts)
		}()
	}

	go func() {
		wg.Wait()
		close(output)
	}()

	return output
}

// testInternal tests a single resource with retry logic using exponential backoff.
// It resolves the resource from the registry and applies the configured retry
// strategy until the resource test passes or max attempts are reached.
func (r *Runner) testInternal(ctx context.Context, resource string, opts Options) error {
	rsc, err := r.registry.Resolve(resource)

	if err != nil {
		return err
	}

	b := backoff.NewExponentialBackOff()
	b.InitialInterval = opts.interval
	b.MaxInterval = opts.maxInterval

	return backoff.Retry(func() error {
		return rsc.Test(ctx)
	}, backoff.WithContext(backoff.WithMaxRetries(b, opts.attempts), ctx))
}
