// Package realtime explicitly mounts the optional realtime admin module.
package realtime

import "github.com/assurrussa/goadmin/host"

func New(stream host.EventStream) host.Module { return host.RealtimeModule(stream) }
