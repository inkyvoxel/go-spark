package jobs

import (
	"context"
	"time"
)

const DefaultEmailInterval = 5 * time.Second

func NewEmailJob(processPending func(context.Context) error, interval time.Duration) Job {
	return Job{
		Name:       "email-outbox",
		Interval:   interval,
		RunAtStart: true,
		Run:        processPending,
	}
}
