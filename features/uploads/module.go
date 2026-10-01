// Package uploads explicitly mounts the optional uploads admin module.
package uploads

import "github.com/assurrussa/goadmin/host"

type Config = host.UploadsConfig

func New(cfg Config) host.Module { return host.UploadsModule(cfg) }
