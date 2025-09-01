package waitfor

import "errors"

// ErrWait is returned when resource availability testing fails.
// This error indicates that one or more resources did not become
// available within the configured timeout and retry parameters.
var (
	ErrWait            = errors.New("failed to wait for resource availability")
	// ErrInvalidArgument is returned when invalid arguments are passed
	// to functions, such as empty resource URLs or invalid configuration.
	ErrInvalidArgument = errors.New("invalid argument")
)
