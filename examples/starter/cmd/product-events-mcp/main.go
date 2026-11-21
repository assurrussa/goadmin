// product-events-mcp starts the shared stdio client for the starter journal.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/assurrussa/goadmin/features/mcp"
)

func main() {
	origin := flag.String("admin-url", os.Getenv("GOADMIN_URL"), "starter origin, e.g. http://localhost:8080")
	flag.Parse()
	if err := mcp.RunStdio(context.Background(), *origin, "/agent/product-events"); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
