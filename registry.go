package waitfor

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Registry supports concurrent registration and inspection during setup. The
// runner freezes registration on its first valid wait. Resolve and List remain
// available after freezing. A Registry must not be copied after first use.
type Registry struct {
	mu        sync.RWMutex
	resources map[string]ResourceFactory
	frozen    bool
}

func newRegistry(configs []ResourceConfig) (*Registry, error) {
	r := &Registry{}
	for _, config := range configs {
		if len(config.Scheme) == 0 {
			return nil, fmt.Errorf("%w: resource config must contain a scheme", ErrInvalidArgument)
		}
		for _, scheme := range config.Scheme {
			if err := r.Register(scheme, config.Factory); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

// Register adds a factory during setup. Schemes are trimmed and lowercased.
// Invalid schemes or nil factories return ErrInvalidArgument; duplicates return
// ErrResourceAlreadyRegistered. Once frozen, all registrations return
// ErrRegistryFrozen. Failed registrations leave the registry unchanged.
func (r *Registry) Register(scheme string, factory ResourceFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frozen {
		return ErrRegistryFrozen
	}

	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if !validScheme(scheme) {
		return fmt.Errorf("%w: invalid resource scheme %q", ErrInvalidArgument, scheme)
	}

	if factory == nil {
		return fmt.Errorf("%w: nil factory for %s", ErrInvalidArgument, scheme)
	}

	if _, exists := r.resources[scheme]; exists {
		return fmt.Errorf("%w: %s", ErrResourceAlreadyRegistered, scheme)
	}

	if r.resources == nil {
		r.resources = make(map[string]ResourceFactory)
	}

	r.resources[scheme] = factory

	return nil
}

// Resolve creates a resource without freezing the registry. The selected factory
// runs outside the registry lock. URL parsing and factory errors are preserved.
func (r *Registry) Resolve(location string) (Resource, error) {
	return resolveResource(location, func(scheme string) ResourceFactory {
		r.mu.RLock()
		defer r.mu.RUnlock()

		return r.resources[scheme]
	})
}

// List returns an independent, sorted slice of registered schemes.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]string, 0, len(r.resources))
	for scheme := range r.resources {
		list = append(list, scheme)
	}

	sort.Strings(list)

	return list
}

// freeze returns the now-immutable mapping for lock-free waiting reads.
func (r *Registry) freeze() map[string]ResourceFactory {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.frozen = true

	return r.resources
}
