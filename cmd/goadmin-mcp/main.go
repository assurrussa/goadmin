// goadmin-mcp serves one configured read resource or confirmed action over local MCP stdio.
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
	path := flag.String("resource-path", os.Getenv("GOADMIN_MCP_RESOURCE_PATH"), "configured MCP resource or confirmed action path")
	flag.Parse()
	if err := mcp.RunStdio(context.Background(), *origin, *path); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
