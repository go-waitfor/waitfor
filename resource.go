package waitfor

import (
	"context"
	"fmt"
	"net/url"
)

type (
	// ResourceFactory creates a usable, non-nil resource from a URL. Factories must
	// return promptly and support concurrent calls, including for duplicate URLs.
	ResourceFactory func(u *url.URL) (Resource, error)

	// ResourceConfig maps one or more URL schemes to a factory.
	ResourceConfig struct {
		Scheme  []string
		Factory ResourceFactory
	}

	// Resource tests availability. Test returns nil when ready and must honor
	// cancellation. All errors from Test are retryable until a configured boundary.
	// Factories returning shared resources must make their Test calls concurrency-safe.
	Resource interface {
		Test(ctx context.Context) error
	}

	// ResourceFunc adapts a function to Resource.
	ResourceFunc func(context.Context) error
)

// resolveResource performs no locking while invoking the selected factory.
func resolveResource(location string, lookup func(string) ResourceFactory) (Resource, error) {
	u, err := url.Parse(location)
	if err != nil {
		return nil, err
	}

	factory := lookup(u.Scheme)
	if factory == nil {
		return nil, fmt.Errorf("%w: %s", ErrResourceNotFound, u.Scheme)
	}

	resource, err := factory(u)
	if err != nil {
		return nil, err
	}

	if resource == nil {
		return nil, fmt.Errorf("%w: factory for %s returned a nil resource", ErrInvalidArgument, u.Scheme)
	}

	return resource, nil
}

// Test invokes fn with the supplied context.
func (fn ResourceFunc) Test(ctx context.Context) error {
	return fn(ctx)
}
