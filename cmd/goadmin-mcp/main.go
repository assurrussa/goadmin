// goadmin-mcp serves one configured goadmin resource over local MCP stdio.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/assurrussa/goadmin/features/mcp"
)

func main() {
	origin := flag.String("admin-url", os.Getenv("GOADMIN_URL"), "admin origin, e.g. https://admin.example.com")
	path := flag.String("resource-path", os.Getenv("GOADMIN_MCP_RESOURCE_PATH"), "configured read-only MCP resource path")
	flag.Parse()
	if err := mcp.RunStdio(context.Background(), *origin, *path); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
