// Command magpie is the MagpieMail server: a single binary that serves the API,
// runs background workers, migrates the database and administers the instance.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/veHRz/MagpieMail-Server/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], os.Environ(), os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
