# Signal Context

[![GoDoc](https://img.shields.io/badge/go-documentation-blue.svg?style=flat-square)](https://pkg.go.dev/mod/github.com/sethvargo/go-signalcontext)
[![GitHub Actions](https://img.shields.io/github/workflow/status/sethvargo/go-signalcontext/Test?style=flat-square)](https://github.com/sethvargo/go-signalcontext/actions?query=workflow%3ATest)

Signal Context (`signalcontext`) is a Go library for creating Go context's that
cancel on signals. This is very useful for client-side applications that want to cancel operations on user interrupts (e.g. CTRL+C).

## Features

- **Small** - A very tiny API and less than 50 lines of code.

- **Independent** - No external dependencies besides the Go standard library,
  meaning it won't bloat your project.

- **Flexible** - Use native Go contexts and extend/wrap as needed.

## Usage

Here is an example for gracefully stopping an HTTP server when the user presses
CTRL+C in their terminal:

```golang
package main

import (
	"context"
	"log"
	"net/http"
	"syscall"
	"time"

	"github.com/sethvargo/go-signalcontext"
)

func main() {
	ctx, cancel := signalcontext.OnInterrupt()
	defer cancel()

	s := &http.Server{
		Addr: ":8080",
	}
	go func() {
		if err := s.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait for CTRL+C
	<-ctx.Done()

	// Stop the server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(shutdownCtx); err != nil {
		log.Fatal(err)
	}
}
```

You can also use custom signals:

```golang
ctx, cancel := signalcontext.On(syscall.SIGUSR1)
defer cancel()
```

And also wrap an existing context:

```golang
ctx1, cancel1 := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel1()

ctx2, cancel2 := signalcontext.Wrap(ctx1, syscall.SIGUSR1)
defer cancel2()
```
