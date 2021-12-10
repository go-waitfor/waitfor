# waitfor
> Test and wait on the availability of a remote resource.

## Features
- Parallel availability tests
- Exponential backoff
- Different types of remote resource (http(s), proc, postgres, mysql)

## Resources
- File (``file://``)
- OS Process (``proc://``)
- HTTP(S) Endpoint (``http://`` & ``https://``)
- MongoDB (``mongodb://``)
- Postgres (``postgres://``)
- MySQL/MariaDB (``mysql://`` & ``mariadb://``)

## Resource URLs
All resource locations start with url schema type e.g. ``file://./myfile`` or ``postgres://locahost:5432/mydb?user=user&password=test``

## Library

### Test resource availability

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


### Test resource availability and run a program
``waitfor`` can be helpful to run external commands with dependencies.     
It makes sure that any program's dependencies are ready and then executes a given command.

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

### Extend
Starting v2, ``waitfor`` allows register custom resource assertions:

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

## CLI
CLI is a simple wrapper around this library.

### Basic usage
```bash
    waitfor -r postgres://locahost:5432/mydb?user=user&password=test -r http://myservice:8080 npm start
```

### Options
```bash
NAME:
   waitfor - Tests and waits on the availability of a remote resource

USAGE:
   waitfor [global options] command [command options] [arguments...]

DESCRIPTION:
   Tests and waits on the availability of a remote resource before executing a command with exponential backoff

COMMANDS:
   help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --resource value, -r value  -r http://localhost:8080 [$WAITFOR_RESOURCE]
   --attempts value, -a value  amount of attempts (default: 5) [$WAITFOR_ATTEMPTS]
   --interval value            interval between attempts (sec) (default: 5) [$WAITFOR_INTERVAL]
   --max-interval value        maximum interval between attempts (sec) (default: 60) [$WAITFOR_MAX_INTERVAL]
   --help, -h                  show help (default: false)
   --version, -v               print the version (default: false)

```
