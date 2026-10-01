// Package queues explicitly mounts the optional queues admin module.
package queues

import "github.com/assurrussa/goadmin/host"

func New() host.Module { return host.QueuesModule() }
