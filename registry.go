package waitfor

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

type (
	ResourceFactory func(u *url.URL) (Resource, error)

	ResourceConfig struct {
		Scheme  []string
		Factory ResourceFactory
	}

	Resource interface {
		Test(ctx context.Context) error
	}

	Registry struct {
		resources map[string]ResourceFactory
	}
)

func newRegistry(configs []ResourceConfig) *Registry {
	resources := make(map[string]ResourceFactory)

	for _, c := range configs {
		for _, s := range c.Scheme {
			resources[s] = c.Factory
		}
	}

	return &Registry{resources}
}

// Register adds a resource factory to the registry
func (r *Registry) Register(scheme string, factory ResourceFactory) error {
	scheme = strings.TrimSpace(scheme)
	_, exists := r.resources[scheme]

	if exists {
		return errors.New("resource is already registered with a given scheme:" + scheme)
	}

	r.resources[scheme] = factory

	return nil
}

// Resolve returns a resource instance by a given url
func (r *Registry) Resolve(location string) (Resource, error) {
	u, err := url.Parse(location)

	if err != nil {
		return nil, err
	}

	rf, found := r.resources[u.Scheme]

	if !found {
		return nil, errors.New("resource with a given scheme is not found:" + u.Scheme)
	}

	return rf(u)
}

// List returns a list of schemes of registered resources
func (r *Registry) List() []string {
	list := make([]string, 0, len(r.resources))

	for k := range r.resources {
		list = append(list, k)
	}

	return list
}
