# DataGrid implementation

This directory contains goadmin-owned implementation code. Host applications
should import `github.com/assurrussa/goadmin/toolkit/datagrid`, not this package.

See the [public toolkit guide](../../../toolkit/datagrid/README.md) for current
repository signatures, configuration, routes, transport constraints and error
handling. Its [executable example](../../../toolkit/datagrid/example_test.go)
is compile-checked and output-checked by `go test ./toolkit/datagrid`.

Implementation tests remain here; public contract/security tests live in
`toolkit/datagrid`. Run `make check` for the repository's canonical verification.
