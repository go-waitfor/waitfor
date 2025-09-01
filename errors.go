package waitfor

import "errors"

var (
	// ErrWait is returned when resource availability testing fails.
	// This error indicates that one or more resources did not become
	// available within the configured timeout and retry parameters.
	ErrWait = errors.New("failed to wait for resource availability")
	// ErrInvalidArgument is returned when invalid arguments are passed
	// to functions, such as empty resource URLs or invalid configuration.
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrResourceAlreadyRegistered is returned when a resource factory is already registered for a scheme.
	ErrResourceAlreadyRegistered = errors.New("resource is already registered with a given scheme")
	// ErrResourceNotFound is returned when no resource factory is found for a scheme.
	ErrResourceNotFound = errors.New("resource with a given scheme is not found")
)
