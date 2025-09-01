package waitfor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewOptions_Defaults(t *testing.T) {
	opts := newOptions([]Option{})

	assert.Equal(t, time.Duration(5)*time.Second, opts.interval)
	assert.Equal(t, time.Duration(60)*time.Second, opts.maxInterval)
	assert.Equal(t, uint64(5), opts.attempts)
}

func TestNewOptions_WithSetters(t *testing.T) {
	setters := []Option{
		WithInterval(10),
		WithMaxInterval(120),
		WithAttempts(15),
	}

	opts := newOptions(setters)

	assert.Equal(t, time.Duration(10)*time.Second, opts.interval)
	assert.Equal(t, time.Duration(120)*time.Second, opts.maxInterval)
	assert.Equal(t, uint64(15), opts.attempts)
}

func TestWithInterval(t *testing.T) {
	option := WithInterval(30)
	opts := &options{}

	option(opts)

	assert.Equal(t, time.Duration(30)*time.Second, opts.interval)
}

func TestWithMaxInterval(t *testing.T) {
	option := WithMaxInterval(90)
	opts := &options{}

	option(opts)

	assert.Equal(t, time.Duration(90)*time.Second, opts.maxInterval)
}

func TestWithAttempts(t *testing.T) {
	option := WithAttempts(20)
	opts := &options{}

	option(opts)

	assert.Equal(t, uint64(20), opts.attempts)
}

func TestCombinedOptions(t *testing.T) {
	opts := newOptions([]Option{
		WithInterval(2),
		WithMaxInterval(30),
		WithAttempts(8),
	})

	assert.Equal(t, time.Duration(2)*time.Second, opts.interval)
	assert.Equal(t, time.Duration(30)*time.Second, opts.maxInterval)
	assert.Equal(t, uint64(8), opts.attempts)
}

func TestWithMultiplier(t *testing.T) {
	opts := newOptions([]Option{
		WithMultiplier(2.5),
	})

	assert.Equal(t, 2.5, opts.multiplier)
}

func TestWithRandomizationFactor(t *testing.T) {
	opts := newOptions([]Option{
		WithRandomizationFactor(0.3),
	})

	assert.Equal(t, 0.3, opts.randomizationFactor)
}
