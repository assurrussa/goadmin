package outbox

import infraoutbox "github.com/assurrussa/goadmin/infrastructure/outbox"

type Register interface {
	MustRegisterJob(job infraoutbox.Job)
	RegisterJob(job infraoutbox.Job) error
}
