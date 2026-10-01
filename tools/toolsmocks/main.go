package main

import (
	"context"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// A thin wrapper over go.uber.org/mock/mockgen with sensible defaults.
// Usage in code, if global install:
//
//	//go:generate toolsmocks
//
// Usage in code, if local project install:
//
//	//go:generate toolsmocks
//
// Or with overrides:
//
//	//go:generate mockgen -destination=mocks/usecase_mock.gen.go -package=${GOPACKAGE}mocks . InterfaceName
func main() {
	gofile := os.Getenv("GOFILE")
	gopackage := os.Getenv("GOPACKAGE")

	var (
		src  string
		out  string
		pkg  string
		args string
	)
	flag.StringVar(&src, "source", "", "source file to scan (default: $GOFILE)")
	flag.StringVar(&out, "destination", "", "output file (default: mocks/<base>_mock.gen.go)")
	flag.StringVar(&pkg, "package", "", "package name for mocks (default: ${GOPACKAGE}mocks)")
	flag.StringVar(&args, "args", "", "extra args to pass to mockgen")
	flag.Parse()

	if src == "" {
		src = gofile
	}
	if src == "" {
		log.Fatal("source not set and GOFILE is empty; run via go:generate or pass -source")
	}

	if pkg == "" {
		if gopackage == "" {
			log.Fatal("GOPACKAGE not set; pass -package explicitly")
		}
		pkg = gopackage + "mocks"
	}

	if out == "" {
		base := strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
		out = filepath.Join("mocks", base+"_mock.gen.go")
	}

	// Parse the file to find all interfaces
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, src, nil, 0)
	if err != nil {
		//nolint:gosec // internal code-generation tool
		log.Fatalf("failed to parse %s: %v", src, err)
	}

	var interfaces []string
	ast.Inspect(node, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok {
			return true
		}
		if _, ok := ts.Type.(*ast.InterfaceType); ok {
			interfaces = append(interfaces, ts.Name.Name)
		}
		return true
	})

	if len(interfaces) == 0 {
		//nolint:gosec // internal code-generation tool
		log.Printf("no interfaces found in %s, skipping mock generation", src)
		return
	}

	cmdArgs := []string{
		"-destination=" + out,
		"-package=" + pkg,
	}

	if args != "" {
		extra := strings.Fields(strings.ReplaceAll(args, ",", " "))
		cmdArgs = append(cmdArgs, extra...)
	}

	// Use reflect mode: mockgen [options] package interfaces...
	cmdArgs = append(cmdArgs, ".", strings.Join(interfaces, ","))

	//nolint:gosec // internal code-generation tool
	log.Println("//go:generate mockgen", strings.Join(cmdArgs, " "))

	ctx := context.Background()
	//nolint:gosec // internal code-generation tool
	cmd := exec.CommandContext(ctx, "mockgen", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}
