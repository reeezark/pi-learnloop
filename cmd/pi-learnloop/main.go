package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/reeezark/pi-learnloop/internal/daemon"
)

// version is set only by the release builder; source builds remain diagnostic dev builds.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 1 && arguments[0] == "version" {
		fmt.Fprintf(stdout, "pi-learnloop %s\n", version)
		return 0
	}
	if len(arguments) != 1 || arguments[0] != "daemon" {
		fmt.Fprintln(stderr, "usage: pi-learnloop daemon | version")
		return 2
	}
	if err := daemon.Run(ctx, daemon.Config{}); err != nil {
		fmt.Fprintf(stderr, "pi-learnloop daemon: %v\n", err)
		return 1
	}
	return 0
}
