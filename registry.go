package waitfor

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

type (
	// ResourceFactory is a function type that creates Resource instances from URLs.
	// Each factory is responsible for parsing the URL and creating the appropriate
	// resource implementation that can test the specific resource type.
	//
	// Example:
	//
	//	func httpResourceFactory(u *url.URL) (Resource, error) {
	//		return &HTTPResource{url: u}, nil
	//	}
	ResourceFactory func(u *url.URL) (Resource, error)

	// ResourceConfig defines the configuration for a resource type, mapping
	// URL schemes to their corresponding factory functions. This allows
	// the system to support multiple resource types through a plugin-like architecture.
	//
	// Example:
	//
	//	config := ResourceConfig{
	//		Scheme:  []string{"http", "https"},
	//		Factory: httpResourceFactory,
	//	}
	ResourceConfig struct {
		Scheme  []string        // URL schemes this config handles (e.g., "http", "postgres")
		Factory ResourceFactory // Factory function to create resource instances
	}

	// Resource defines the interface that all resource types must implement.
	// The Test method should verify that the resource is available and ready,
	// returning an error if the resource is not accessible or not ready.
	//
	// Implementations should be context-aware and respect cancellation signals.
	Resource interface {
		// Test verifies that the resource is available and ready for use.
		// Should return nil if the resource is ready, or an error describing
		// why the resource is not available.
		Test(ctx context.Context) error
	}

	// Registry manages the mapping between URL schemes and their corresponding
	// resource factories. It provides methods to register new resource types
	// and resolve URLs to resource instances.
	Registry struct {
		resources map[string]ResourceFactory
	}
)

// newRegistry creates a new Registry instance and populates it with the provided
// resource configurations. Each configuration maps one or more URL schemes to
// their corresponding factory functions.
func newRegistry(configs []ResourceConfig) *Registry {
	resources := make(map[string]ResourceFactory)

	for _, c := range configs {
		for _, s := range c.Scheme {
			resources[s] = c.Factory
		}
	}

	return &Registry{resources}
}

// Register adds a resource factory to the registry for the specified URL scheme.
// The scheme is automatically trimmed of whitespace. Returns an error if a
// factory is already registered for the given scheme.
//
// Example:
//
//	err := registry.Register("custom", myResourceFactory)
//	if err != nil {
//		// Handle registration conflict
//	}
func (r *Registry) Register(scheme string, factory ResourceFactory) error {
	scheme = strings.TrimSpace(scheme)
	_, exists := r.resources[scheme]

	if exists {
		return fmt.Errorf("%w: %s", ErrResourceAlreadyRegistered, scheme)
	}

	r.resources[scheme] = factory

	return nil
}

// Resolve parses the location URL and creates a Resource instance using the
// appropriate factory for the URL's scheme. Returns an error if the URL
// cannot be parsed or if no factory is registered for the scheme.
//
// Example:
//
//	resource, err := registry.Resolve("postgres://user:pass@localhost:5432/db")
//	if err != nil {
//		// Handle resolution error
//	}
//	err = resource.Test(ctx)
func (r *Registry) Resolve(location string) (Resource, error) {
	u, err := url.Parse(location)

	if err != nil {
		return nil, err
	}

	rf, found := r.resources[u.Scheme]

	if !found {
		return nil, fmt.Errorf("%w: %s", ErrResourceNotFound, u.Scheme)
	}

	return rf(u)
}

// List returns a slice containing all registered URL schemes.
// The order of schemes in the returned slice is not guaranteed.
// This can be useful for debugging or displaying available resource types.
func (r *Registry) List() []string {
	list := make([]string, 0, len(r.resources))

	for k := range r.resources {
		list = append(list, k)
	}

	return list
}
