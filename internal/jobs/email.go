package jobs

import (
	"context"
	"time"
)

const DefaultEmailInterval = 5 * time.Second

type EmailProcessor interface {
	ProcessPending(context.Context) error
}

func NewEmailJob(processor EmailProcessor, interval time.Duration) Job {
	return Job{
		Name:       "email-outbox",
		Interval:   interval,
		RunAtStart: true,
		Run:        processor.ProcessPending,
	}
}
