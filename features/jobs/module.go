// Package jobs explicitly mounts the optional jobs admin module.
package jobs

import "github.com/assurrussa/goadmin/host"

type Config = host.JobsConfig

func New(cfg Config) host.Module { return host.JobsModule(cfg) }
