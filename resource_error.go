package waitfor

import (
	"fmt"
	"net/url"
)

// ResourceError associates a failed resource with its original error.
type ResourceError struct {
	Resource string
	Err      error
}

// Error identifies the resource without printing URL credentials or query values.
// For sensitive locations the underlying error may also contain the URL, so its
// text is omitted. Resource and Err retain their original values.
func (e ResourceError) Error() string {
	u, err := url.Parse(e.Resource)
	if err != nil {
		return "invalid resource URL"
	}

	// net/url accepts relative paths, including mistyped credential-bearing
	// URLs. No resource can resolve without a registered scheme.
	if u.Scheme == "" {
		return "resource URL has no scheme"
	}

	if u.Opaque != "" {
		return u.Scheme + ": resource check failed"
	}

	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		u.User = nil
		u.RawQuery, u.Fragment = "", ""
		u.ForceQuery = false

		return u.String() + ": resource check failed"
	}

	return fmt.Sprintf("%s: %v", e.Resource, e.Err)
}

// Unwrap preserves the original error for errors.Is and errors.As.
func (e ResourceError) Unwrap() error {
	return e.Err
}
