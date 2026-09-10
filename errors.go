package waitfor

import (
	"errors"
	"fmt"
	"strings"
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

// ResourceError associates a failed resource with its original error.
type ResourceError struct {
	Resource string
	Err      error
}

// Error identifies the resource and describes its failure.
func (e ResourceError) Error() string {
	return fmt.Sprintf("%s: %v", e.Resource, e.Err)
}

// Unwrap preserves the original error for errors.Is and errors.As.
func (e ResourceError) Unwrap() error {
	return e.Err
}

// WaitError aggregates failed resources in input order.
type WaitError struct {
	Errors []ResourceError
}

// Error describes all failed resources in deterministic input order.
func (e *WaitError) Error() string {
	var message strings.Builder
	message.WriteString(ErrWait.Error())
	for i, failure := range e.Errors {
		if i == 0 {
			message.WriteString(": ")
		} else {
			message.WriteString("; ")
		}
		message.WriteString(failure.Error())
	}
	return message.String()
}

// Unwrap exposes ErrWait and each resource failure for errors.Is and errors.As.
func (e *WaitError) Unwrap() []error {
	errs := make([]error, 1, len(e.Errors)+1)
	errs[0] = ErrWait
	for _, failure := range e.Errors {
		errs = append(errs, failure)
	}
	return errs
}
