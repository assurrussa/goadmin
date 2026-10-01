// Package notifications explicitly mounts the optional notifications admin module.
package notifications

import "github.com/assurrussa/goadmin/host"

func New(manager host.NotificationManager) host.Module { return host.NotificationsModule(manager) }
