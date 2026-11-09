# waitfor

[![Build Status](https://github.com/go-waitfor/waitfor/workflows/Build/badge.svg)](https://github.com/go-waitfor/waitfor/actions)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-waitfor/waitfor/v2.svg)](https://pkg.go.dev/github.com/go-waitfor/waitfor/v2)

Wait for resources concurrently before proceeding or starting a command. The core
provides retry limits, duration-based exponential delays, inspectable errors,
per-resource results, and an optional observer. It has no logging or telemetry
dependency. Resource integrations live in separate packages.

## Installation

```sh
go get github.com/go-waitfor/waitfor/v2
```

Existing v1 consumers keep the `github.com/go-waitfor/waitfor` import path and
their v1 release dependency. Adopting v2 requires an explicit import-path change.

Existing integrations include [HTTP](https://github.com/go-waitfor/waitfor-http),
[files](https://github.com/go-waitfor/waitfor-fs),
[processes](https://github.com/go-waitfor/waitfor-proc),
[PostgreSQL](https://github.com/go-waitfor/waitfor-postgres),
[MySQL](https://github.com/go-waitfor/waitfor-mysql), and
[MongoDB](https://github.com/go-waitfor/waitfor-mongodb).
Integration releases whose `Use()` helpers return a v1 `ResourceConfig` cannot
pass those configurations directly to v2 `New`. They must migrate their core
imports and factories to `github.com/go-waitfor/waitfor/v2` before use with v2.
Integration and CLI migrations are separate from this core module change.

## Quick start

This self-contained example uses an always-ready custom resource. Replace the
function body with your context-aware readiness check.

```go
package main

import (
    "context"
    "fmt"
    "net/url"
    "time"

    "github.com/go-waitfor/waitfor/v2"
)

func main() {
    runner, err := waitfor.New(waitfor.ResourceConfig{
        Scheme: []string{"ready"},
        Factory: func(*url.URL) (waitfor.Resource, error) {
            return waitfor.ResourceFunc(func(context.Context) error {
                return nil
            }), nil
        },
    })
    if err != nil {
        fmt.Println(err)
        return
    }

    err = runner.Test(
        context.Background(),
        []string{"ready://a", "ready://b"},
        waitfor.WithAttempts(0),
        waitfor.WithInterval(500*time.Millisecond),
        waitfor.WithMaxInterval(5*time.Second),
        waitfor.WithMaxElapsedTime(30*time.Second),
    )
    if err != nil {
        fmt.Println(err)
    }
}
```

## What ends a wait

Each resource is resolved once and tested as soon as its worker starts. By
default, all resources can run concurrently. Use `WithMaxConcurrency(n)` to
process up to `n` resources at a time per wait. Each resource holds its slot
through resolution, tests, retry delays, and observer calls. With unlimited
attempts, an unavailable resource can hold its slot until cancellation. Queued
resources share the wait deadline. If canceled before starting, they report zero attempts.

A resource finishes on success, a resolution error, a permanent probe error,
attempt exhaustion, or cancellation. Its siblings continue independently. Return
`waitfor.Permanent(err)` from a probe when retrying cannot help, for example
after an authentication failure. The marker works through wrapping and preserves
the original error for `errors.Is` and `errors.As`. `Permanent(nil)` returns nil.

`WithAttempts(0)` permits unlimited attempts. Retryable failures continue
until success or cancellation by the caller or the configured elapsed limit.

A positive `WithMaxElapsedTime` creates one deadline shared by all resources,
starting before dispatch. The earlier of this deadline and the caller's deadline
wins. Attempt and elapsed limits can both be set; the first reached stops that
resource. Cancellation observed after an attempt takes precedence over its
success or exhausted attempt limit. A context already canceled at entry performs
no resource tests.

Cancellation is cooperative. Factories must construct resources promptly.
`Resource.Test` must honor the supplied context, and observers must return
promptly. The library waits for those calls to return and does not abandon
goroutines to enforce deadlines. A valid empty resource list succeeds, including
with a canceled context.

## Retry options

| Option | Default | Meaning |
| --- | --- | --- |
| `WithAttempts(uint64)` | `5` | Total test calls per resource, including the first; `0` is unlimited |
| `WithMaxConcurrency(int)` | `0` | Maximum active resources per wait. Zero is unlimited. |
| `WithInterval(time.Duration)` | `5*time.Second` | Initial exponential base interval |
| `WithMaxInterval(time.Duration)` | `60*time.Second` | Maximum actual retry delay, including jitter |
| `WithMaxElapsedTime(time.Duration)` | `0` | Shared wait deadline; `0` disables this limit |
| `WithMultiplier(float64)` | `1.5` | Growth of the base interval; `1` keeps it constant |
| `WithRandomizationFactor(float64)` | `0.5` | Jitter factor; `0` disables jitter |
| `WithAttemptObserver(func(Attempt))` | `nil` | Optional retry notification |

Intervals use durations directly, with no conversion from seconds. The base
interval grows exponentially; jitter varies it by the configured factor above or
below the base, and the resulting delay is capped at the maximum interval.
Every resource has independent backoff state. A multiplier above one increases
the base by at least one nanosecond per retry until it reaches the cap.

Options are applied in order, with the last valid setter winning. Each supplied
value is validated when applied, so an invalid value remains an error even if a
later option replaces it. Relationships between final values are checked after
successful application, before registry freezing or resource work.

Intervals must be positive, the maximum must be at least the initial interval,
the multiplier must be finite and at least one, jitter must be finite and within
`[0, 1]`, and elapsed limits must not be negative. Attempt limits must fit in
`int`; timing values whose jitter calculation could overflow `time.Duration`
are rejected. Nil options are ignored; a nil observer disables observation.

Invalid options return an error wrapping `ErrInvalidOption`, without `ErrWait`.
Independent validation failures are joined, and named validation errors from
`github.com/ziflex/go-options` remain inspectable with `errors.As`. Invalid
options are never retried.
No backoff-library types are required in the public API, and backoff-specific
error types receive no special handling.

## Results and errors

`Test(ctx, resources, ...Option) error` is the convenience form of:

```go
func (r *Runner) Check(ctx context.Context, resources []string, opts ...Option) ([]Result, error)

type Result struct {
    Resource string
    Attempts int
    Duration time.Duration
    Err      error
}

type ResourceError struct {
    Resource string
    Err      error
}

type WaitError struct {
    Errors []ResourceError
}
```

Valid calls return one result per input, in input order, including duplicates.
A duplicate URL is resolved and checked independently. `Attempts` counts actual
`Resource.Test` calls; resolution failures have zero attempts. `Duration` measures
resolution, tests, delays, and observation after the worker starts. Successful
results have `Err == nil`. Invalid retry options return nil results and a
configuration error.

Resource failures return the results together with a `*WaitError`. It retains
`ErrWait` and ordered `ResourceError` entries in its error tree. Attempt exhaustion
preserves the final original resource error, rather than collecting every
intermediate failure. On cancellation, the last failure is joined with the context
error and any distinct cancellation cause. Completed successes remain successful
if another resource subsequently times out.

Formatted wait errors omit URL userinfo, query strings, and fragments. For
sensitive or opaque URLs they also omit the underlying error text, which can
repeat credentials. Malformed URLs produce a generic message. The original
location and probe error are available in the result, observer event, and
`ResourceError` fields. Use those fields only where the raw values are needed.
For ordinary URLs, probes should return error messages suitable for logging.

```go
results, err := runner.Check(ctx, resources, waitfor.WithAttempts(3))
for i, result := range results {
    fmt.Printf("resource[%d] attempts=%d duration=%s ready=%t\n",
        i, result.Attempts, result.Duration, result.Err == nil)
}

if errors.Is(err, waitfor.ErrInvalidOption) {
    // Fix the retry configuration; no resources were tested.
}
if errors.Is(err, context.DeadlineExceeded) {
    // Either the caller deadline or the explicit elapsed limit expired.
}
var failures *waitfor.WaitError
if errors.As(err, &failures) {
    for _, failure := range failures.Errors {
        fmt.Println(failure)
    }
}
// errors.Is and errors.As also reach the original resource errors.
```

Other sentinels are `ErrInvalidArgument` for invalid resource configuration,
`ErrResourceAlreadyRegistered` for duplicate schemes,
`ErrResourceNotFound` for unknown schemes, and `ErrRegistryFrozen` for registration
after waiting begins. URL parser and resource factory errors retain their chains.

## Observing retries

```go
err := runner.Test(ctx, resources,
    waitfor.WithAttemptObserver(func(a waitfor.Attempt) {
        failure := waitfor.ResourceError{Resource: a.Resource, Err: a.Err}
        fmt.Printf("attempt %d failed (%v), retry in %s\n", a.Number, failure, a.Next)
    }),
)
```

The observer runs after a failed attempt when another retry is planned.
`Number` starts at one, `Err` is the original failure, and `Next` includes jitter
and the actual delay cap. No notification is sent for successes, resolution
failures, permanent errors, exhausted attempts, observed cancellation, or delays
that cannot fit before the deadline. A planned retry may still be canceled
before it executes.

Callbacks run synchronously and are serialized across resources within a single
wait. Their cross-resource order is unspecified; results and aggregate errors
remain ordered. Callbacks must return promptly, and their duration counts toward
the elapsed limit. Modifying the callback's value argument does not change retry
policy. Callers sharing callback state across separate concurrent waits must
synchronize it. Callbacks may update caller-owned logs or metrics without adding
those dependencies to waitfor.

## Custom resources and registry lifecycle

```go
type Resource interface {
    Test(context.Context) error
}

type ResourceFactory func(*url.URL) (Resource, error)

type ResourceConfig struct {
    Scheme  []string
    Factory ResourceFactory
}
```

Use `ResourceFunc` for an ad-hoc check:

```go
runner, err := waitfor.New(waitfor.ResourceConfig{
    Scheme: []string{"custom"},
    Factory: func(u *url.URL) (waitfor.Resource, error) {
        return waitfor.ResourceFunc(func(ctx context.Context) error {
            return checkDependency(ctx, u)
        }), nil
    },
})
```

`checkDependency` above is the caller's context-aware readiness function.

All construction errors return a nil runner. `New()` creates an empty registry.
During setup, `runner.Resources().Register(scheme, factory)` may add more factories.
Schemes are trimmed, validated as ASCII URL schemes, and lowercased. They must be
unique both within and across configurations. Empty configurations and nil
factories are invalid; factories must return usable, non-nil resources.

The first valid `Check`, `Test`, `Run`, or `RunTo` freezes registration
permanently, even with an empty resource list or an already canceled context.
Invalid retry options do not freeze it. Every subsequent registration returns
`ErrRegistryFrozen`.

Registry operations are synchronized. Registration racing the first wait either
completes before freezing and becomes visible to that wait, or fails as frozen.
`List()` returns a sorted independent slice. `Resolve(location)` remains available
after freezing and does not itself freeze registration. Factories execute outside
registry locks and must support concurrent calls. Shared resource instances must
also support concurrent tests. A registry must not be copied after first use.

## Waiting before running a command

```go
output, err := runner.Run(ctx, waitfor.Program{
    Executable: "myapp",
    Args:       []string{"--config", "app.yaml"},
    Resources:  []string{"ready://a"},
}, waitfor.WithMaxElapsedTime(30*time.Second))
```

`Run` starts the command after all resources succeed and returns its combined
stdout and stderr. For long-running or verbose commands, `RunTo` streams both
to a single writer as output arrives. Writes are serialized. A nil writer
discards output.

```go
program := waitfor.Program{Executable: "myapp", WaitDelay: 2 * time.Second}
err := runner.RunTo(ctx, program, os.Stdout)
```

Both methods use the caller's context for the command. `WithMaxElapsedTime`
applies only to readiness. Canceling the caller's context kills the direct
child, which can produce an `*exec.ExitError`. By default, output collection
waits for EOF, even if a descendant still holds a pipe after the child exits.

A positive `Program.WaitDelay` sets `exec.Cmd.WaitDelay`. The timer starts on
context cancellation or direct-child exit, whichever comes first. When it
expires, Go can close inherited output pipes and return `exec.ErrWaitDelay`
if the child otherwise succeeded. Descendants can continue running. Streaming
writers must return promptly. A negative wait delay fails with
`ErrInvalidArgument` before the readiness checks.

## Breaking changes and migration

- V1 uses `github.com/go-waitfor/waitfor`; v2 uses
  `github.com/go-waitfor/waitfor/v2`. Existing v1 release tags retain the v1 API.
  To upgrade after publication, run `go get github.com/go-waitfor/waitfor/v2`,
  update every core import to `/v2`, and apply the API changes below. The package
  name remains `waitfor`; source stays at the repository root and the intended
  first stable release tag is `v2.0.0`.
- Handle both return values from `New`: `runner, err := waitfor.New(...)`.
  Duplicate configurations now fail instead of silently overwriting factories.
- Replace integer-second options with explicit durations:
  `WithInterval(2*time.Second)` and `WithMaxInterval(30*time.Second)`.
  Bare integer literals now denote nanoseconds. Typed integer variables require
  an explicit conversion; there is no compatibility conversion inside waitfor.
- For the adjacent CLI, handle constructor errors and migrate its interval flags
  to durations, or explicitly convert its existing seconds at the CLI boundary.
  The CLI repository is not changed by this library refactor.
- Move registration before the first wait. Runtime registration after that point
  returns `ErrRegistryFrozen`.
- Use keyed fields in `Program` literals. The new `WaitDelay` field defaults to zero.
- Replace core `Module` and `waitfor.Use(module)` with a direct `ResourceConfig`.
  Resource-package helpers such as `http.Use()` must return v2 configurations;
  helpers returning v1 configurations are incompatible with v2 `New`.
  The `ResourceConfig.Scheme` field and `Resource` interface shape are unchanged,
  but the module paths give v1 and v2 their own type identities.
- Replace string matching on wait errors with `errors.Is`/`errors.As`. Error text
  now identifies resources in input order, and cancellation preserves error chains.
- Retry interval, multiplier, and jitter settings now take effect. Unlimited
  attempts have no hidden elapsed limit; configure a caller context or explicit
  `WithMaxElapsedTime` when a bound is required. Invalid settings fail before work.

## Development

Development requires Go 1.26 or later. CI tests Go 1.26.0 and the current stable release.

```sh
go test ./...
go test -race ./...
go vet ./...
revive -config revive.toml -formatter stylish -exclude ./vendor/... ./...
```

Tests use controlled resources and a helper subprocess, without external services.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
