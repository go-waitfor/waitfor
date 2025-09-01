# waitfor

[![Build Status](https://github.com/go-waitfor/waitfor/workflows/Build/badge.svg)](https://github.com/go-waitfor/waitfor/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/go-waitfor/waitfor)](https://goreportcard.com/report/github.com/go-waitfor/waitfor)
[![GoDoc](https://godoc.org/github.com/go-waitfor/waitfor?status.svg)](https://godoc.org/github.com/go-waitfor/waitfor)
[![Go Version](https://img.shields.io/github/go-mod/go-version/go-waitfor/waitfor)](https://github.com/go-waitfor/waitfor)

> Test and wait on the availability of remote resources before proceeding with your application logic.

`waitfor` is a Go library that provides a robust way to test and wait for remote resource availability with built-in retry logic, exponential backoff, and extensible resource support. It's particularly useful for ensuring dependencies are ready before starting applications or running critical operations.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Supported Resources](#supported-resources)
- [Resource URLs](#resource-urls)
- [Quick Start](#quick-start)
  - [Test Resource Availability](#test-resource-availability)
  - [Test and Run Program](#test-and-run-program)
  - [Custom Resource Types](#custom-resource-types)
- [API Reference](#api-reference)
  - [Core Types](#core-types)
  - [Functions](#functions)
  - [Configuration Options](#configuration-options)
- [Advanced Usage](#advanced-usage)
- [Error Handling](#error-handling)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

## Features

- **Parallel Testing**: Test multiple resources concurrently for faster startup times
- **Exponential Backoff**: Smart retry logic that prevents overwhelming resources
- **Extensible Architecture**: Support for custom resource types through a plugin system
- **Context Support**: Full context support for cancellation and timeouts
- **Zero Dependencies**: Minimal external dependencies for easy integration
- **Production Ready**: Battle-tested retry logic with configurable parameters

## Installation

Install `waitfor` using Go modules:

```bash
go get github.com/go-waitfor/waitfor
```

For specific resource types, install the corresponding packages:

```bash
# Database resources
go get github.com/go-waitfor/waitfor-postgres
go get github.com/go-waitfor/waitfor-mysql

# File system and process resources  
go get github.com/go-waitfor/waitfor-fs
go get github.com/go-waitfor/waitfor-proc

# HTTP resources
go get github.com/go-waitfor/waitfor-http

# NoSQL databases
go get github.com/go-waitfor/waitfor-mongodb
```

## Supported Resources

The following resource types are available through separate packages:

| Resource Type | Package | URL Schemes | Description |
|---------------|---------|-------------|-------------|
| [File System](https://github.com/go-waitfor/waitfor-fs) | `waitfor-fs` | `file://` | Test file/directory existence |
| [OS Process](https://github.com/go-waitfor/waitfor-proc) | `waitfor-proc` | `proc://` | Test process availability |
| [HTTP(S) Endpoint](https://github.com/go-waitfor/waitfor-http) | `waitfor-http` | `http://`, `https://` | Test HTTP endpoint availability |
| [PostgreSQL](https://github.com/go-waitfor/waitfor-postgres) | `waitfor-postgres` | `postgres://` | Test PostgreSQL database connectivity |
| [MySQL/MariaDB](https://github.com/go-waitfor/waitfor-mysql) | `waitfor-mysql` | `mysql://`, `mariadb://` | Test MySQL/MariaDB connectivity |
| [MongoDB](https://github.com/go-waitfor/waitfor-mongodb) | `waitfor-mongodb` | `mongodb://` | Test MongoDB connectivity |

## Resource URLs

Resource locations are specified using standard URL format with scheme-specific parameters:

**Format**: `scheme://[user[:password]@]host[:port][/path][?query]`

**Examples**:
- `file://./myfile` - Local file path
- `file:///absolute/path/to/file` - Absolute file path
- `http://localhost:8080/health` - HTTP health check endpoint
- `https://api.example.com/status` - HTTPS endpoint with path
- `postgres://user:password@localhost:5432/mydb` - PostgreSQL database
- `mysql://user:password@localhost:3306/mydb` - MySQL database
- `mongodb://localhost:27017/mydb` - MongoDB database
- `proc://nginx` - Process by name

## Quick Start

### Test Resource Availability

Use `waitfor` to test if resources are available before proceeding:

```go
package main

import (
	"context"
	"fmt"
	"github.com/go-waitfor/waitfor"
	"github.com/go-waitfor/waitfor-postgres"
	"os"
)

func main() {
	runner := waitfor.New(postgres.Use())

	err := runner.Test(
		context.Background(),
		[]string{"postgres://locahost:5432/mydb?user=user&password=test"},
		waitfor.WithAttempts(5),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
```


### Test and Run Program

`waitfor` can ensure dependencies are ready before executing external commands, making it perfect for application startup scripts and deployment scenarios:

```go
package main

import (
	"context"
	"fmt"
	"github.com/go-waitfor/waitfor"
	"github.com/go-waitfor/waitfor-postgres"
	"os"
)

func main() {
	runner := waitfor.New(postgres.Use())

	program := waitfor.Program{
		Executable: "myapp",
		Args:       []string{"--database", "postgres://locahost:5432/mydb?user=user&password=test"},
		Resources:  []string{"postgres://locahost:5432/mydb?user=user&password=test"},
	}

	out, err := runner.Run(
		context.Background(),
		program,
		waitfor.WithAttempts(5),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	fmt.Println(string(out))
}
```

### Custom Resource Types

`waitfor` supports custom resource types through its extensible registry system. You can register your own resource checkers:

```go
package main

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq"
	"github.com/go-waitfor/waitfor"
	"net/url"
	"os"
	"strings"
)

const PostgresScheme = "postgres"

type PostgresResource struct {
	url *url.URL
}

func (p *PostgresResource) Test(ctx context.Context) error {
	db, err := sql.Open(p.url.Scheme, strings.TrimPrefix(p.url.String(), PostgresScheme+"://"))

	if err != nil {
		return err
	}

	defer db.Close()

	return db.PingContext(ctx)
}

func main() {
	runner := waitfor.New(waitfor.ResourceConfig{
		Scheme: []string{PostgresScheme},
		Factory: func(u *url.URL) (waitfor.Resource, error) {
			return &PostgresResource{u}, nil
		},
	})

	err := runner.Test(
		context.Background(),
		[]string{"postgres://locahost:5432/mydb?user=user&password=test"},
		waitfor.WithAttempts(5),
	)

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
```

## API Reference

### Core Types

#### Runner

The main entry point for testing resources and running programs.

```go
type Runner struct {
    // registry contains all registered resource factories
}
```

#### Program

Defines an external command with its dependencies.

```go
type Program struct {
    Executable string   // Command to execute
    Args       []string // Command arguments
    Resources  []string // Dependencies to test before execution
}
```

#### Resource

Interface that all resource types must implement.

```go
type Resource interface {
    Test(ctx context.Context) error
}
```

#### ResourceConfig

Configuration for registering resource types.

```go
type ResourceConfig struct {
    Scheme  []string        // URL schemes this resource handles
    Factory ResourceFactory // Factory function to create resource instances
}
```

#### ResourceFactory

Function signature for creating resource instances.

```go
type ResourceFactory func(u *url.URL) (Resource, error)
```

### Functions

#### New

Creates a new Runner with the specified resource configurations.

```go
func New(configurators ...ResourceConfig) *Runner
```

**Parameters:**
- `configurators`: Variable number of ResourceConfig instances to register

**Returns:** A new Runner instance

**Example:**
```go
runner := waitfor.New(postgres.Use(), http.Use())
```

#### (*Runner) Test

Tests the availability of specified resources.

```go
func (r *Runner) Test(ctx context.Context, resources []string, setters ...Option) error
```

**Parameters:**
- `ctx`: Context for cancellation and timeout control
- `resources`: Slice of resource URLs to test
- `setters`: Configuration options (WithAttempts, WithInterval, etc.)

**Returns:** Error if any resource is unavailable after all retry attempts

#### (*Runner) Run

Tests resources and executes a program if all resources are available.

```go
func (r *Runner) Run(ctx context.Context, program Program, setters ...Option) ([]byte, error)
```

**Parameters:**
- `ctx`: Context for cancellation and timeout control
- `program`: Program configuration with executable, args, and resource dependencies
- `setters`: Configuration options

**Returns:** Combined stdout/stderr output and error

#### (*Runner) Resources

Returns the resource registry for advanced usage.

```go
func (r *Runner) Resources() *Registry
```

**Returns:** The internal Registry instance

#### Use

Helper function to convert module functions to ResourceConfig.

```go
func Use(mod Module) ResourceConfig
```

**Parameters:**
- `mod`: Module function that returns schemes and factory

**Returns:** ResourceConfig ready for use with New()

### Configuration Options

All test and run operations accept configuration options to customize behavior:

#### WithAttempts

Sets the maximum number of retry attempts.

```go
func WithAttempts(attempts uint64) Option
```

**Default:** 5 attempts

**Example:**
```go
err := runner.Test(ctx, resources, waitfor.WithAttempts(10))
```

#### WithInterval

Sets the initial retry interval in seconds.

```go
func WithInterval(interval uint64) Option
```

**Default:** 5 seconds

**Example:**
```go
err := runner.Test(ctx, resources, waitfor.WithInterval(2))
```

#### WithMaxInterval

Sets the maximum retry interval for exponential backoff in seconds.

```go
func WithMaxInterval(interval uint64) Option
```

**Default:** 60 seconds

**Example:**
```go
err := runner.Test(ctx, resources, waitfor.WithMaxInterval(120))
```

#### Combining Options

Options can be combined for fine-tuned control:

```go
err := runner.Test(
    ctx, 
    resources,
    waitfor.WithAttempts(15),
    waitfor.WithInterval(1),
    waitfor.WithMaxInterval(30),
)
```

## Advanced Usage

### Multiple Resource Types

Test different types of resources simultaneously:

```go
package main

import (
    "context"
    "fmt"
    "github.com/go-waitfor/waitfor"
    "github.com/go-waitfor/waitfor-postgres"
    "github.com/go-waitfor/waitfor-http"
    "github.com/go-waitfor/waitfor-fs"
)

func main() {
    runner := waitfor.New(
        postgres.Use(),
        http.Use(),
        fs.Use(),
    )
    
    resources := []string{
        "postgres://user:pass@localhost:5432/mydb",
        "http://localhost:8080/health",
        "file://./config.json",
    }
    
    err := runner.Test(context.Background(), resources)
    if err != nil {
        fmt.Printf("Dependencies not ready: %v\n", err)
        return
    }
    
    fmt.Println("All dependencies are ready!")
}
```

### Context Cancellation and Timeouts

Use context for timeout control and cancellation:

```go
package main

import (
    "context"
    "time"
    "github.com/go-waitfor/waitfor"
    "github.com/go-waitfor/waitfor-postgres"
)

func main() {
    runner := waitfor.New(postgres.Use())
    
    // Set a 30-second timeout
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    err := runner.Test(
        ctx,
        []string{"postgres://localhost:5432/mydb"},
        waitfor.WithAttempts(10),
    )
    
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            fmt.Println("Timeout waiting for resources")
        } else {
            fmt.Printf("Resource test failed: %v\n", err)
        }
        return
    }
    
    fmt.Println("Resources are ready!")
}
```

### Dynamic Resource Registration

Register resources at runtime:

```go
package main

import (
    "context"
    "net/url"
    "github.com/go-waitfor/waitfor"
)

func main() {
    runner := waitfor.New()
    
    // Register a custom resource type
    err := runner.Resources().Register("custom", func(u *url.URL) (waitfor.Resource, error) {
        return &MyCustomResource{url: u}, nil
    })
    
    if err != nil {
        panic(err)
    }
    
    // Now you can use the custom resource
    err = runner.Test(context.Background(), []string{"custom://example.com"})
    // ... handle error
}
```

## Error Handling

`waitfor` provides specific error types for different failure scenarios:

### Error Types

- `ErrWait`: Returned when resources are not available after all retry attempts
- `ErrInvalidArgument`: Returned for invalid input parameters

### Error Handling Patterns

```go
err := runner.Test(ctx, resources)
if err != nil {
    // Check if it's a waitfor-specific error
    if strings.Contains(err.Error(), waitfor.ErrWait.Error()) {
        fmt.Println("Resources are not available after retries")
        // Maybe wait longer or use different configuration
    } else {
        fmt.Printf("Configuration or setup error: %v\n", err)
    }
    return
}
```

### Timeout vs Resource Failure

```go
ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
defer cancel()

err := runner.Test(ctx, resources)
if err != nil {
    if ctx.Err() == context.DeadlineExceeded {
        fmt.Println("Overall timeout exceeded")
    } else {
        fmt.Println("Resource-specific failure:", err)
    }
}
```

## Best Practices

### 1. Choose Appropriate Timeouts

Set timeouts based on your application's requirements:

```go
// For quick startup (development)
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

// For production startup
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
```

### 2. Configure Retry Behavior

Adjust retry parameters based on resource characteristics:

```go
// For fast resources (local files, processes)
waitfor.WithAttempts(3),
waitfor.WithInterval(1),
waitfor.WithMaxInterval(5)

// For slow resources (remote databases, external APIs)
waitfor.WithAttempts(15),
waitfor.WithInterval(5),
waitfor.WithMaxInterval(60)
```

### 3. Group Related Resources

Test related resources together for better error reporting:

```go
// Test database cluster
databaseResources := []string{
    "postgres://localhost:5432/primary",
    "postgres://localhost:5433/replica1", 
    "postgres://localhost:5434/replica2",
}

// Test web services
webResources := []string{
    "http://localhost:8080/health",
    "http://localhost:8081/ready",
}

// Test separately for clearer error messages
if err := runner.Test(ctx, databaseResources); err != nil {
    log.Fatal("Database cluster not ready:", err)
}

if err := runner.Test(ctx, webResources); err != nil {
    log.Fatal("Web services not ready:", err)
}
```

### 4. Use Structured Logging

Integrate with structured logging for better observability:

```go
logger := log.With().Str("component", "waitfor").Logger()

logger.Info().Msg("Starting dependency checks")

err := runner.Test(ctx, resources)
if err != nil {
    logger.Error().Err(err).Msg("Dependencies not ready")
    return
}

logger.Info().Msg("All dependencies ready")
```

## Troubleshooting

### Common Issues

#### "resource with a given scheme is not found"

This error occurs when you try to use a resource type that hasn't been registered.

**Solution**: Import and register the appropriate resource package:

```go
import "github.com/go-waitfor/waitfor-postgres"

runner := waitfor.New(postgres.Use())
```

#### "failed to wait for resource availability"

This indicates that resources were not available after all retry attempts.

**Solutions**:
1. Increase retry attempts: `waitfor.WithAttempts(20)`
2. Increase retry intervals: `waitfor.WithMaxInterval(120)`
3. Check if the resource URL is correct
4. Verify the resource is actually running and accessible

#### Connection Refused or Timeout Errors

These are typically network-related issues.

**Solutions**:
1. Verify the resource is running: `telnet hostname port`
2. Check firewall rules and network connectivity
3. Verify DNS resolution for hostnames
4. Use IP addresses instead of hostnames if DNS is an issue

### Debug Mode

Enable verbose error reporting for troubleshooting:

```go
err := runner.Test(ctx, resources)
if err != nil {
    fmt.Printf("Detailed error: %+v\n", err)
    
    // Test each resource individually to isolate issues
    for _, resource := range resources {
        if testErr := runner.Test(ctx, []string{resource}); testErr != nil {
            fmt.Printf("Failed resource: %s - %v\n", resource, testErr)
        }
    }
}
```

### Performance Considerations

#### Parallel vs Sequential Testing

By default, `waitfor` tests all resources in parallel for faster execution. For resource-constrained environments, consider testing sequentially:

```go
// Test resources one by one
for _, resource := range resources {
    err := runner.Test(ctx, []string{resource})
    if err != nil {
        return fmt.Errorf("resource %s failed: %w", resource, err)
    }
}
```

#### Memory Usage

When testing many resources, be aware of goroutine overhead. For very large numbers of resources (100+), consider batching:

```go
const batchSize = 10

for i := 0; i < len(resources); i += batchSize {
    end := i + batchSize
    if end > len(resources) {
        end = len(resources)
    }
    
    batch := resources[i:end]
    err := runner.Test(ctx, batch)
    if err != nil {
        return err
    }
}
```

## Contributing

We welcome contributions! Here's how you can help:

### Adding New Resource Types

1. Create a new repository following the pattern `waitfor-{resourcetype}`
2. Implement the `Resource` interface:
   ```go
   type MyResource struct {
       url *url.URL
   }
   
   func (r *MyResource) Test(ctx context.Context) error {
       // Implement your resource test logic
       return nil
   }
   ```
3. Provide a `Use()` function:
   ```go
   func Use() waitfor.ResourceConfig {
       return waitfor.ResourceConfig{
           Scheme: []string{"myscheme"},
           Factory: func(u *url.URL) (waitfor.Resource, error) {
               return &MyResource{url: u}, nil
           },
       }
   }
   ```

### Reporting Issues

When reporting issues, please include:
- Go version
- `waitfor` version  
- Resource types and URLs being tested
- Complete error messages
- Minimal reproduction case

### Development Setup

```bash
git clone https://github.com/go-waitfor/waitfor.git
cd waitfor
go mod tidy
go test ./...
```

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.