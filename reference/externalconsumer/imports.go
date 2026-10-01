package externalconsumer

import (
	// required feature.
	_ "github.com/assurrussa/goadmin/features/access"
	_ "github.com/assurrussa/goadmin/features/authmail"
	_ "github.com/assurrussa/goadmin/features/jobs"
	_ "github.com/assurrussa/goadmin/features/notifications"
	_ "github.com/assurrussa/goadmin/features/operations"
	_ "github.com/assurrussa/goadmin/features/queues"
	_ "github.com/assurrussa/goadmin/features/realtime"
	_ "github.com/assurrussa/goadmin/features/uploads"
	_ "github.com/assurrussa/goadmin/features/users"
	_ "github.com/assurrussa/goadmin/host"
	_ "github.com/assurrussa/goadmin/hosttest"
	_ "github.com/assurrussa/goadmin/migrations"
	_ "github.com/assurrussa/goadmin/toolkit/datagrid"
	_ "github.com/assurrussa/goadmin/toolkit/formvalidator"
)
