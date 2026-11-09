package waitfor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readyFactory(*url.URL) (Resource, error) {
	return ResourceFunc(func(context.Context) error { return nil }), nil
}

func TestNewRegistryValidation(t *testing.T) {
	runner, err := New()
	require.NoError(t, err)
	assert.Empty(t, runner.Resources().List())
	for _, tc := range []struct {
		name    string
		configs []ResourceConfig
		want    error
	}{
		{"empty config", []ResourceConfig{{Factory: readyFactory}}, ErrInvalidArgument},
		{"nil factory", []ResourceConfig{{Scheme: []string{"test"}}}, ErrInvalidArgument},
		{"same config duplicate", []ResourceConfig{{Scheme: []string{"test", "test"}, Factory: readyFactory}}, ErrResourceAlreadyRegistered},
		{"normalized duplicate", []ResourceConfig{{Scheme: []string{" test ", "TEST"}, Factory: readyFactory}}, ErrResourceAlreadyRegistered},
		{"cross config duplicate", []ResourceConfig{
			{Scheme: []string{"test"}, Factory: readyFactory},
			{Scheme: []string{"test", "another"}, Factory: readyFactory},
		}, ErrResourceAlreadyRegistered},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner, err := New(tc.configs...)
			assert.Nil(t, runner)
			require.ErrorIs(t, err, tc.want)
		})
	}
	for _, scheme := range []string{"", " ", "1test", "+test", "te st", "test://", "test_", "té"} {
		t.Run("invalid "+scheme, func(t *testing.T) {
			runner, err := New(ResourceConfig{Scheme: []string{scheme}, Factory: readyFactory})
			assert.Nil(t, runner)
			require.ErrorIs(t, err, ErrInvalidArgument)
			registry := &Registry{}
			require.ErrorIs(t, registry.Register(scheme, readyFactory), ErrInvalidArgument)
			assert.Empty(t, registry.List())
		})
	}
}

func TestRegistrySetupAndIndependentList(t *testing.T) {
	schemes := []string{" HTTPS ", "http", "Custom+v1.2-test"}
	config := ResourceConfig{Scheme: schemes, Factory: readyFactory}
	runner, err := New(config)
	require.NoError(t, err)
	registry := runner.Resources()
	schemes[0] = "changed"
	config.Factory = nil
	require.Equal(t, []string{"custom+v1.2-test", "http", "https"}, registry.List())
	list := registry.List()
	list[0] = "changed"
	assert.Equal(t, []string{"custom+v1.2-test", "http", "https"}, registry.List())
	resource, err := registry.Resolve("HTTPS://localhost")
	require.NoError(t, err)
	require.NoError(t, resource.Test(context.Background()))
	require.ErrorIs(t, registry.Register(" HTTP ", readyFactory), ErrResourceAlreadyRegistered)
	require.ErrorIs(t, registry.Register("new", nil), ErrInvalidArgument)
	require.NoError(t, registry.Register("new", readyFactory))
	require.NoError(t, runner.Test(context.Background(), []string{"new://a"}))
	for _, scheme := range []string{"http", "another", ""} {
		require.ErrorIs(t, registry.Register(scheme, readyFactory), ErrRegistryFrozen)
	}
	assert.Equal(t, []string{"custom+v1.2-test", "http", "https", "new"}, registry.List())
}

func TestRegistryFreezesOnEveryWaitEntryPoint(t *testing.T) {
	for _, method := range []string{"Check", "Test", "Run"} {
		t.Run(method, func(t *testing.T) {
			runner, err := New()
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Run must not launch anything, but still freezes setup.
			switch method {
			case "Check":
				_, err = runner.Check(ctx, nil)
				require.NoError(t, err)
			case "Test":
				require.NoError(t, runner.Test(ctx, nil))
			case "Run":
				_, err = runner.Run(ctx, helperProgram(t, "echo", ""))
				require.ErrorIs(t, err, context.Canceled)
			}
			require.ErrorIs(t, runner.Resources().Register("late", readyFactory), ErrRegistryFrozen)
		})
	}
}

func TestRegistryFactoryRunsWithoutLock(t *testing.T) {
	registry := &Registry{}
	require.NoError(t, registry.Register("test", func(*url.URL) (Resource, error) {
		// Registration from the factory would deadlock under a held read lock.
		if err := registry.Register("nested", readyFactory); err != nil {
			return nil, err
		}
		return readyFactory(nil)
	}))
	done := make(chan error, 1)
	go func() { _, err := registry.Resolve("test://a"); done <- err }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-testContext(t).Done():
		t.Fatal("factory ran under registry lock")
	}
	assert.Equal(t, []string{"nested", "test"}, registry.List())
}

func TestRegistryConcurrentSetupReads(t *testing.T) {
	registry := &Registry{}
	require.NoError(t, registry.Register("base", readyFactory))
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, registry.Register(fmt.Sprintf("scheme%d", i), readyFactory))
			resource, err := registry.Resolve("base://a")
			assert.NoError(t, err)
			assert.NotNil(t, resource)
			assert.Contains(t, registry.List(), "base")
		}()
	}
	wg.Wait()
	assert.Len(t, registry.List(), 21)
}

func TestRegistryConcurrentFreezeAndRegistration(t *testing.T) {
	for range 30 {
		runner, err := New()
		require.NoError(t, err)
		start := make(chan struct{})
		registered, checked := make(chan error, 1), make(chan error, 1)
		ctx := testContext(t)
		go func() {
			<-start
			registered <- runner.Resources().Register("test", readyFactory)
		}()
		go func() {
			<-start
			checked <- runner.Test(ctx, []string{"test://a"}, WithAttempts(1))
		}()
		close(start)
		registrationErr, checkErr := <-registered, <-checked
		if registrationErr == nil {
			require.NoError(t, checkErr)
			assert.Equal(t, []string{"test"}, runner.Resources().List())
		} else {
			require.ErrorIs(t, registrationErr, ErrRegistryFrozen)
			require.ErrorIs(t, checkErr, ErrResourceNotFound)
			assert.Empty(t, runner.Resources().List())
		}
		require.ErrorIs(t, runner.Resources().Register("late", readyFactory), ErrRegistryFrozen)
	}
}

func TestRegistryPreservesFactoryErrors(t *testing.T) {
	failure := errors.New("factory failure")
	runner := testRunner(t, func(*url.URL) (Resource, error) { return nil, failure })
	resource, err := runner.Resources().Resolve("test://a")
	assert.Nil(t, resource)
	assert.Same(t, failure, err)
	resource, err = runner.Resources().Resolve("://bad")
	assert.Nil(t, resource)
	var parseError *url.Error
	assert.ErrorAs(t, err, &parseError)
}

func TestRegistryRejectsUnicodeSchemeBeforeFolding(t *testing.T) {
	var registry Registry
	require.ErrorIs(t, registry.Register("FİLE", readyFactory), ErrInvalidArgument)
	require.NoError(t, registry.Register(" FILE ", readyFactory))
	_, err := registry.Resolve("FiLe://host/path")
	require.NoError(t, err)
}

type valueResource struct{}

func (valueResource) Test(context.Context) error { return nil }

func TestFactoryResourceValues(t *testing.T) {
	for _, tc := range []struct {
		name     string
		resource Resource
		valid    bool
	}{
		{"nil function", ResourceFunc(nil), false},
		{"nil pointer", (*valueResource)(nil), false},
		{"value", valueResource{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := testRunner(t, func(*url.URL) (Resource, error) { return tc.resource, nil })
			resource, err := runner.Resources().Resolve("test://host")
			results, checkErr := runner.Check(testContext(t), []string{"test://host"})
			if tc.valid {
				require.NoError(t, err)
				require.Equal(t, tc.resource, resource)
				require.NoError(t, checkErr)
				require.Equal(t, 1, results[0].Attempts)
				return
			}
			require.Nil(t, resource)
			require.ErrorIs(t, err, ErrInvalidArgument)
			require.ErrorIs(t, checkErr, ErrInvalidArgument)
			require.Zero(t, results[0].Attempts)
		})
	}
}
