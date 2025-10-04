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

// QueueDetail provides an extended view of a queue, including raw attributes and tags.
type QueueDetail struct {
	QueueSummary
	Arn            string
	LastModifiedAt time.Time
	Attributes     map[string]string
	Tags           map[string]string
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

// MessageAttribute represents a single name/value pair returned with a message.
type MessageAttribute struct {
	Name  string
	Value string
}

// SendMessageInput carries the parameters necessary to enqueue a message.
type SendMessageInput struct {
	QueueURL       string
	Body           string
	MessageGroupID string
	DelaySeconds   *int32
	Attributes     []MessageAttribute
}

// ReceiveMessagesInput controls how messages are fetched from a queue.
type ReceiveMessagesInput struct {
	QueueURL            string
	MaxMessages         int32
	WaitTimeSeconds     int32
	MaxMessagesProvided bool
	WaitTimeProvided    bool
}

// ReceiveMessagesResult contains the messages retrieved from a queue.
type ReceiveMessagesResult struct {
	Messages []ReceivedMessage
}

// DeleteMessageInput carries the parameters required to remove a message from a queue.
type DeleteMessageInput struct {
	QueueURL      string
	ReceiptHandle string
}

// ReceivedMessage represents a single message retrieved from SQS.
type ReceivedMessage struct {
	ID            string
	Body          string
	ReceiptHandle string
	ReceiveCount  int32
	Attributes    []MessageAttribute
}
