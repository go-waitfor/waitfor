package waitfor

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestResource struct {
	calls int
}

func (t *TestResource) Test(_ context.Context) error {
	t.calls++
	return nil
}

func TestRegistry_Register(t *testing.T) {
	resolutions := make([]*url.URL, 0, 10)

	r := newRegistry([]ResourceConfig{
		{
			Scheme: []string{"http", "https"},
			Factory: func(u *url.URL) (Resource, error) {
				resolutions = append(resolutions, u)

				return &TestResource{}, nil
			},
		},
	})

	rsc, err := r.Resolve("http://localhost:8080")

	assert.NoError(t, err)
	assert.NotNilf(t, rsc, "resource not found")
}

func TestRegistry_Register_NewScheme(t *testing.T) {
	r := newRegistry([]ResourceConfig{})
	
	factory := func(_ *url.URL) (Resource, error) {
		return &TestResource{}, nil
	}
	
	err := r.Register("custom", factory)
	assert.NoError(t, err)
	
	// Verify the scheme was registered
	rsc, err := r.Resolve("custom://test")
	assert.NoError(t, err)
	assert.NotNil(t, rsc)
}

func TestRegistry_Register_DuplicateScheme(t *testing.T) {
	factory := func(_ *url.URL) (Resource, error) {
		return &TestResource{}, nil
	}
	
	r := newRegistry([]ResourceConfig{
		{
			Scheme:  []string{"existing"},
			Factory: factory,
		},
	})
	
	// Try to register the same scheme again
	err := r.Register("existing", factory)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource is already registered with a given scheme:")
}

func TestRegistry_Register_WithWhitespace(t *testing.T) {
	r := newRegistry([]ResourceConfig{})
	
	factory := func(_ *url.URL) (Resource, error) {
		return &TestResource{}, nil
	}
	
	// Register with whitespace - should be trimmed
	err := r.Register("  spaced  ", factory)
	assert.NoError(t, err)
	
	// Verify it was registered with trimmed name
	rsc, err := r.Resolve("spaced://test")
	assert.NoError(t, err)
	assert.NotNil(t, rsc)
}

func TestRegistry_Resolve_InvalidURL(t *testing.T) {
	r := newRegistry([]ResourceConfig{})
	
	// Test with invalid URL
	rsc, err := r.Resolve("://invalid-url")
	assert.Error(t, err)
	assert.Nil(t, rsc)
}

func TestRegistry_Resolve_UnknownScheme(t *testing.T) {
	r := newRegistry([]ResourceConfig{})
	
	// Test with unknown scheme
	rsc, err := r.Resolve("unknown://test")
	assert.Error(t, err)
	assert.Nil(t, rsc)
	assert.Contains(t, err.Error(), "resource with a given scheme is not found:")
}

func TestRegistry_Resolve_FactoryError(t *testing.T) {
	factory := func(_ *url.URL) (Resource, error) {
		return nil, errors.New("factory error")
	}
	
	r := newRegistry([]ResourceConfig{
		{
			Scheme:  []string{"error"},
			Factory: factory,
		},
	})
	
	rsc, err := r.Resolve("error://test")
	assert.Error(t, err)
	assert.Nil(t, rsc)
	assert.Contains(t, err.Error(), "factory error")
}

func TestRegistry_List(t *testing.T) {
	factory := func(_ *url.URL) (Resource, error) {
		return &TestResource{}, nil
	}
	
	r := newRegistry([]ResourceConfig{
		{
			Scheme:  []string{"http", "https", "custom"},
			Factory: factory,
		},
	})
	
	schemes := r.List()
	assert.Len(t, schemes, 3)
	assert.Contains(t, schemes, "http")
	assert.Contains(t, schemes, "https")
	assert.Contains(t, schemes, "custom")
}

func TestRegistry_List_Empty(t *testing.T) {
	r := newRegistry([]ResourceConfig{})
	
	schemes := r.List()
	assert.Empty(t, schemes)
}
