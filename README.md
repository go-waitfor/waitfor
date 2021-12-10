# waitfor
> Test and wait on the availability of a remote resource.

## Features
- Parallel availability tests.
- Exponential backoff.
- Extensibility. Different types of remote resource (``http(s)``, ``proc``, ``postgres``, ``mysql``).

## Resources
- [File](https://github.com/go-waitfor/waitfor-fs) (``file://``)
- [OS Process](https://github.com/go-waitfor/waitfor-proc) (``proc://``)
- [HTTP(S) Endpoint](https://github.com/go-waitfor/waitfor-http) (``http://`` & ``https://``)
- [MongoDB](https://github.com/go-waitfor/waitfor-mongodb) (``mongodb://``)
- [Postgres](https://github.com/go-waitfor/waitfor-postgres) (``postgres://``)
- [MySQL/MariaDB](https://github.com/go-waitfor/waitfor-mysql) (``mysql://`` & ``mariadb://``)

## Resource URLs
All resource locations start with url schema type e.g. ``file://./myfile`` or ``postgres://locahost:5432/mydb?user=user&password=test``

## Quick start

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