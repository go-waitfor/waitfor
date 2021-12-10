package waitfor

import "errors"

var (
	ErrWait            = errors.New("failed to wait for resource availability")
	ErrInvalidArgument = errors.New("invalid argument")
)
