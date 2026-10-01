package bootstrap

import (
	"errors"

	"github.com/assurrussa/goadmin/infrastructure/outbox"
)

func initOutbox(deps OutboxDependencies, jobsExtra ...outbox.Job) error {
	jobs := make([]outbox.Job, 0, 3+len(jobsExtra))
	if deps.AdminPreviewAttach != nil {
		jobs = append(jobs, deps.AdminPreviewAttach)
	}
	if deps.AdminPreviewDetach != nil {
		jobs = append(jobs, deps.AdminPreviewDetach)
	}
	if deps.AdminNotifyJob != nil {
		jobs = append(jobs, deps.AdminNotifyJob)
	}

	jobs = append(jobs, jobsExtra...)

	if len(jobs) > 0 && deps.Service == nil {
		return errors.New("jobs require jobs module")
	}
	for _, j := range jobs {
		deps.Service.MustRegisterJob(j)
	}

	return nil
}
