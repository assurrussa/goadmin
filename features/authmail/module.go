// Package authmail explicitly mounts the optional authmail admin module.
package authmail

import "github.com/assurrussa/goadmin/host"

type Config = host.AuthMailConfig

func New(cfg Config) host.Module { return host.AuthMailModule(cfg) }
