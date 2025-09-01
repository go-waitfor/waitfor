package waitfor

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestResourceSuccess is a mock resource that always succeeds
type TestResourceSuccess struct {
	calls int
}

func (t *TestResourceSuccess) Test(_ context.Context) error {
	t.calls++
	return nil
}

// TestResourceFailure is a mock resource that always fails
type TestResourceFailure struct {
	calls int
}

func (t *TestResourceFailure) Test(_ context.Context) error {
	t.calls++
	return errors.New("resource not available")
}

// MockResourceFactory creates test resources
func MockResourceFactory(u *url.URL) (Resource, error) {
	if u.Host == "success" {
		return &TestResourceSuccess{}, nil
	}
	if u.Host == "failure" {
		return &TestResourceFailure{}, nil
	}
	return nil, errors.New("unknown host")
}

func TestNew(t *testing.T) {
	// Test with no configurators
	runner := New()
	assert.NotNil(t, runner)
	assert.NotNil(t, runner.registry)

	// Test with configurators
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner = New(config)
	assert.NotNil(t, runner)
	assert.NotNil(t, runner.registry)
}

func TestRunner_Resources(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	registry := runner.Resources()
	assert.NotNil(t, registry)
	assert.Equal(t, runner.registry, registry)
}

func TestRunner_Test_Success(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx := context.Background()
	resources := []string{"test://success"}
	
	err := runner.Test(ctx, resources)
	assert.NoError(t, err)
}

func TestRunner_Test_SingleFailure(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	resources := []string{"test://failure"}
	
	err := runner.Test(ctx, resources, WithAttempts(1))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), ErrWait.Error())
}

func TestRunner_Test_MultipleResources(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx := context.Background()
	
	// Test multiple successful resources
	resources := []string{"test://success", "test://success"}
	err := runner.Test(ctx, resources)
	assert.NoError(t, err)
	
	// Test mix of success and failure with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	resources = []string{"test://success", "test://failure"}
	err = runner.Test(ctx, resources, WithAttempts(1))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), ErrWait.Error())
}

func TestRunner_Test_WithOptions(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx := context.Background()
	resources := []string{"test://success"}
	
	// Test with custom options
	err := runner.Test(ctx, resources, 
		WithAttempts(3),
		WithInterval(1),
		WithMaxInterval(10))
	assert.NoError(t, err)
}

func TestRunner_Test_EmptyResources(t *testing.T) {
	runner := New()
	ctx := context.Background()
	
	// Test with empty resources slice
	err := runner.Test(ctx, []string{})
	assert.NoError(t, err)
}

func TestRunner_Run_Success(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx := context.Background()
	program := Program{
		Executable: "echo",
		Args:       []string{"hello"},
		Resources:  []string{"test://success"},
	}
	
	output, err := runner.Run(ctx, program)
	assert.NoError(t, err)
	assert.Contains(t, string(output), "hello")
}

func TestRunner_Run_ResourceFailure(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	program := Program{
		Executable: "echo",
		Args:       []string{"hello"},
		Resources:  []string{"test://failure"},
	}
	
	output, err := runner.Run(ctx, program, WithAttempts(1))
	assert.Error(t, err)
	assert.Nil(t, output)
	assert.Contains(t, err.Error(), ErrWait.Error())
}

func TestRunner_Run_CommandFailure(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx := context.Background()
	program := Program{
		Executable: "nonexistent-command",
		Args:       []string{},
		Resources:  []string{"test://success"},
	}
	
	_, err := runner.Run(ctx, program)
	assert.Error(t, err)
	// The important thing is that we get an error when the command fails
}

func TestRunner_testInternal_ResolutionError(t *testing.T) {
	runner := New() // No resources registered
	
	ctx := context.Background()
	opts := Options{
		interval:    time.Second,
		maxInterval: time.Minute,
		attempts:    1,
	}
	
	err := runner.testInternal(ctx, "unknown://test", opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "resource with a given scheme is not found")
}

func TestRunner_testInternal_ResourceTestError(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx := context.Background()
	opts := Options{
		interval:    1 * time.Millisecond, // Very short for testing
		maxInterval: 2 * time.Millisecond,
		attempts:    1, // Only one attempt to avoid long test time
	}
	
	err := runner.testInternal(ctx, "test://failure", opts)
	assert.Error(t, err)
}

func TestRunner_testAllInternal(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx := context.Background()
	opts := Options{
		interval:    time.Millisecond,
		maxInterval: time.Millisecond * 10,
		attempts:    1,
	}
	
	// Test with multiple resources
	resources := []string{"test://success", "test://success"}
	output := runner.testAllInternal(ctx, resources, opts)
	
	errorCount := 0
	for err := range output {
		if err != nil {
			errorCount++
		}
	}
	
	assert.Equal(t, 0, errorCount)
}

func TestRunner_testAllInternal_WithErrors(t *testing.T) {
	config := ResourceConfig{
		Scheme:  []string{"test"},
		Factory: MockResourceFactory,
	}
	runner := New(config)
	
	ctx := context.Background()
	opts := Options{
		interval:    time.Millisecond,
		maxInterval: time.Millisecond * 10,
		attempts:    1,
	}
	
	// Test with mix of success and failure
	resources := []string{"test://success", "test://failure"}
	output := runner.testAllInternal(ctx, resources, opts)
	
	errorCount := 0
	for err := range output {
		if err != nil {
			errorCount++
		}
	}
	
	assert.Equal(t, 1, errorCount)
}