package waitfor

import (
	"context"
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
