package waitfor

import (
	"errors"
)

var (
	// ErrWait is returned when resource availability testing fails.
	// This error indicates that one or more resources did not become
	// available within the configured timeout and retry parameters.
	ErrWait = errors.New("failed to wait for resource availability")
	// ErrInvalidArgument identifies invalid resource configuration or a factory
	// that returns a nil resource without an error.
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrInvalidOption identifies invalid retry configuration, before any wait.
	ErrInvalidOption = errors.New("invalid wait option")
	// ErrRegistryFrozen identifies registration attempted after the first wait.
	ErrRegistryFrozen = errors.New("resource registry is frozen")
	// ErrResourceAlreadyRegistered is returned when a resource factory is already registered for a scheme.
	ErrResourceAlreadyRegistered = errors.New("resource is already registered with a given scheme")
	// ErrResourceNotFound is returned when no resource factory is found for a scheme.
	ErrResourceNotFound = errors.New("resource with a given scheme is not found")
)

type (
	// url.Parse errors can repeat credentials in both the URL and the parse reason.
	invalidResourceURL struct {
		cause error
	}

	permanentError struct{ error }
)

func (invalidResourceURL) Error() string {
	return "invalid resource URL"
}

func (e invalidResourceURL) Unwrap() error { return e.cause }

func (e permanentError) Unwrap() error { return e.error }

// Permanent marks a probe failure as terminal for this resource. It preserves
// err for errors.Is and errors.As, even under further wrapping. Permanent(nil)
// returns nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err}
}
