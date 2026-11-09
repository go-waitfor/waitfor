package waitfor

import (
	"context"
	"fmt"
	"net/url"
	"reflect"
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
	// cancellation. Errors are retryable unless marked with Permanent.
	// Factories returning shared resources must make their Test calls concurrency-safe.
	Resource interface {
		Test(ctx context.Context) error
	}

	// ResourceFunc adapts a function to Resource.
	ResourceFunc func(context.Context) error
)

// Test invokes fn with the supplied context.
func (fn ResourceFunc) Test(ctx context.Context) error {
	return fn(ctx)
}

// resolveResource performs no locking while invoking the selected factory.
func resolveResource(location string, lookup func(string) ResourceFactory) (Resource, error) {
	u, err := url.Parse(location)
	if err != nil {
		return nil, invalidResourceURL{err}
	}

	factory := lookup(u.Scheme)
	if factory == nil {
		return nil, fmt.Errorf("%w: %s", ErrResourceNotFound, u.Scheme)
	}

	resource, err := factory(u)
	if err != nil {
		return nil, err
	}

	if nilResource(resource) {
		return nil, fmt.Errorf("%w: factory for %s returned a nil resource", ErrInvalidArgument, u.Scheme)
	}

	return resource, nil
}

func nilResource(resource Resource) bool {
	v := reflect.ValueOf(resource)

	switch v.Kind() {
	case reflect.Invalid:
		return true
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
