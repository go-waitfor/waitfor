package waitfor

import "strings"

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
