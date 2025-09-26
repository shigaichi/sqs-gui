package internal

import "time"

// QueueType represents the queue category (standard or FIFO).
type QueueType string

const (
	// QueueTypeStandard represents a standard queue.
	QueueTypeStandard QueueType = "standard"
	// QueueTypeFIFO represents a FIFO queue.
	QueueTypeFIFO QueueType = "fifo"
)

// QueueSummary aggregates queue statistics for presentation.
type QueueSummary struct {
	URL                       string
	Name                      string
	Type                      QueueType
	CreatedAt                 time.Time
	MessagesAvailable         int64
	MessagesInFlight          int64
	Encryption                string
	ContentBasedDeduplication bool
}

// CreateQueueInput gathers the parameters necessary to create a queue.
type CreateQueueInput struct {
	Name                      string
	Type                      QueueType
	DelaySeconds              *int32
	MessageRetentionPeriod    *int32
	VisibilityTimeout         *int32
	ContentBasedDeduplication bool
}

// CreateQueueResult reports the outcome of a queue creation request.
type CreateQueueResult struct {
	QueueURL string
}
