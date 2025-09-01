package waitfor

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUse(t *testing.T) {
	// Create a mock module function
	mockModule := func() ([]string, ResourceFactory) {
		schemes := []string{"mock", "test"}
		factory := func(_ *url.URL) (Resource, error) {
			return &TestResource{}, nil
		}
		return schemes, factory
	}

	// Test Use function
	config := Use(mockModule)

	assert.Equal(t, []string{"mock", "test"}, config.Scheme)
	assert.NotNil(t, config.Factory)

	// Test that the factory works
	testURL, _ := url.Parse("mock://example")
	resource, err := config.Factory(testURL)
	assert.NoError(t, err)
	assert.NotNil(t, resource)
}

func TestUse_WithEmptySchemes(t *testing.T) {
	// Create a module with empty schemes
	mockModule := func() ([]string, ResourceFactory) {
		schemes := []string{}
		factory := func(_ *url.URL) (Resource, error) {
			return &TestResource{}, nil
		}
		return schemes, factory
	}

	config := Use(mockModule)

	assert.Empty(t, config.Scheme)
	assert.NotNil(t, config.Factory)
}

func TestUse_WithNilFactory(t *testing.T) {
	// Create a module with nil factory
	mockModule := func() ([]string, ResourceFactory) {
		schemes := []string{"nil"}
		return schemes, nil
	}

	config := Use(mockModule)

	assert.Equal(t, []string{"nil"}, config.Scheme)
	assert.Nil(t, config.Factory)
}
