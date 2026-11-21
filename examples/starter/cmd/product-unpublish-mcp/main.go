// product-unpublish-mcp preserves the starter launch command for existing clients.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/assurrussa/goadmin/features/mcp"
)

func main() {
	origin := flag.String("admin-url", os.Getenv("GOADMIN_URL"), "admin origin")
	flag.Parse()
	if err := mcp.RunStdio(context.Background(), *origin, "/agent/product-unpublish"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
