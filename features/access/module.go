// Package access explicitly mounts the optional access admin module.
package access

import "github.com/assurrussa/goadmin/host"

func New() host.Module { return host.AccessModule() }
