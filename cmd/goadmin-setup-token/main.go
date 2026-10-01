package main

import (
	"context"
	"os"

	"github.com/assurrussa/goadmin/internal/setupcmd"
)

func main() {
	os.Exit(setupcmd.Run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}
