//go:build !starter_smoke

package main

import "github.com/assurrussa/goadmin/host"

func starterDashboardModules() []host.Module { return nil }

func starterMailIntegrations(selected map[string]bool) (mailIntegrations, error) {
	return newMailIntegrations(selected)
}
