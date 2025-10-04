package internal

import (
	"context"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

// SqsService encapsulates business logic.
type SqsService interface {
	Queues(ctx context.Context) ([]QueueSummary, error)
	CreateQueue(ctx context.Context, input CreateQueueInput) (CreateQueueResult, error)
	QueueDetail(ctx context.Context, queueURL string) (QueueDetail, error)
	DeleteQueue(ctx context.Context, queueURL string) error
	PurgeQueue(ctx context.Context, queueURL string) error
	SendMessage(ctx context.Context, input SendMessageInput) error
	ReceiveMessages(ctx context.Context, input ReceiveMessagesInput) (ReceiveMessagesResult, error)
	DeleteMessage(ctx context.Context, input DeleteMessageInput) error
}

// SqsServiceImpl is the concrete service implementation.
type SqsServiceImpl struct {
	repo SqsRepository
}

// NewSqsService constructs a new service instance.
func NewSqsService(s SqsRepository) SqsService {
	return &SqsServiceImpl{repo: s}
}

// Queues retrieves queue summaries.
func (s *SqsServiceImpl) Queues(ctx context.Context) ([]QueueSummary, error) {
	return s.repo.ListQueues(ctx)
}

// CreateQueue validates the request and delegates queue creation.
func (s *SqsServiceImpl) CreateQueue(ctx context.Context, input CreateQueueInput) (CreateQueueResult, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CreateQueueResult{}, errors.New("queue name is required")
	}

	queueType := input.Type
	if queueType == "" {
		queueType = QueueTypeStandard
	}

	if queueType == QueueTypeFIFO && !strings.HasSuffix(name, ".fifo") {
		name += ".fifo"
	}

	if queueType == QueueTypeStandard && strings.HasSuffix(name, ".fifo") {
		queueType = QueueTypeFIFO
	}

	if queueType != QueueTypeStandard && queueType != QueueTypeFIFO {
		return CreateQueueResult{}, errors.New("invalid queue type")
	}

	attributes := map[string]string{}

	if input.DelaySeconds != nil {
		attributes["DelaySeconds"] = strconv.FormatInt(int64(*input.DelaySeconds), 10)
	}

	if input.MessageRetentionPeriod != nil {
		attributes["MessageRetentionPeriod"] = strconv.FormatInt(int64(*input.MessageRetentionPeriod), 10)
	}

	if input.VisibilityTimeout != nil {
		attributes["VisibilityTimeout"] = strconv.FormatInt(int64(*input.VisibilityTimeout), 10)
	}

	switch queueType {
	case QueueTypeFIFO:
		attributes["FifoQueue"] = "true"
		if input.ContentBasedDeduplication {
			attributes["ContentBasedDeduplication"] = "true"
		}
	case QueueTypeStandard:
		if input.ContentBasedDeduplication {
			return CreateQueueResult{}, errors.New("content-based deduplication is only available for FIFO queues")
		}
	}

	queueURL, err := s.repo.CreateQueue(ctx, CreateQueueRepositoryInput{
		Name:       name,
		Attributes: attributes,
	})
	if err != nil {
		return CreateQueueResult{}, err
	}

	return CreateQueueResult{QueueURL: queueURL}, nil
}

// QueueDetail returns detailed information for a specific queue URL.
func (s *SqsServiceImpl) QueueDetail(ctx context.Context, queueURL string) (QueueDetail, error) {
	if strings.TrimSpace(queueURL) == "" {
		return QueueDetail{}, errors.New("queue url is required")
	}

	return s.repo.GetQueueDetail(ctx, queueURL)
}

// DeleteQueue deletes the queue identified by queueURL.
func (s *SqsServiceImpl) DeleteQueue(ctx context.Context, queueURL string) error {
	if strings.TrimSpace(queueURL) == "" {
		return errors.New("queue url is required")
	}

	return s.repo.DeleteQueue(ctx, queueURL)
}

// PurgeQueue removes all messages currently stored in the queue.
func (s *SqsServiceImpl) PurgeQueue(ctx context.Context, queueURL string) error {
	if strings.TrimSpace(queueURL) == "" {
		return errors.New("queue url is required")
	}

	return s.repo.PurgeQueue(ctx, queueURL)
}

// SendMessage validates input and delegates to the repository to enqueue a message.
func (s *SqsServiceImpl) SendMessage(ctx context.Context, input SendMessageInput) error {
	queueURL := strings.TrimSpace(input.QueueURL)
	if queueURL == "" {
		return errors.New("queue url is required")
	}

	if strings.TrimSpace(input.Body) == "" {
		return errors.New("message body is required")
	}

	var delay *int32
	if input.DelaySeconds != nil {
		if *input.DelaySeconds < 0 || *input.DelaySeconds > 900 {
			return errors.New("delay seconds must be between 0 and 900")
		}
		delay = input.DelaySeconds
	}

	attributes := make(map[string]string)
	for _, attr := range input.Attributes {
		name := strings.TrimSpace(attr.Name)
		if name == "" {
			continue
		}
		attributes[name] = attr.Value
	}

	return s.repo.SendMessage(ctx, SendMessageRepositoryInput{
		QueueURL:       queueURL,
		Body:           input.Body,
		MessageGroupID: strings.TrimSpace(input.MessageGroupID),
		DelaySeconds:   delay,
		Attributes:     attributes,
	})
}

// ReceiveMessages retrieves messages from SQS applying sensible defaults.
func (s *SqsServiceImpl) ReceiveMessages(ctx context.Context, input ReceiveMessagesInput) (ReceiveMessagesResult, error) {
	queueURL := strings.TrimSpace(input.QueueURL)
	if queueURL == "" {
		return ReceiveMessagesResult{}, errors.New("queue url is required")
	}

	const (
		defaultMaxMessages int32 = 10
		minMaxMessages     int32 = 1
		maxMaxMessages     int32 = 10

		defaultWaitTimeSeconds int32 = 20
		minWaitTimeSeconds     int32 = 0
		maxWaitTimeSeconds     int32 = 20
	)

	maxMessages := input.MaxMessages
	if !input.MaxMessagesProvided {
		maxMessages = defaultMaxMessages
	} else {
		if maxMessages < minMaxMessages {
			maxMessages = minMaxMessages
		} else if maxMessages > maxMaxMessages {
			maxMessages = maxMaxMessages
		}
	}

	waitTime := input.WaitTimeSeconds
	if !input.WaitTimeProvided {
		waitTime = defaultWaitTimeSeconds
	} else {
		if waitTime < minWaitTimeSeconds {
			waitTime = minWaitTimeSeconds
		} else if waitTime > maxWaitTimeSeconds {
			waitTime = maxWaitTimeSeconds
		}
	}

	messages, err := s.repo.ReceiveMessages(ctx, ReceiveMessagesRepositoryInput{
		QueueURL:        queueURL,
		MaxMessages:     maxMessages,
		WaitTimeSeconds: waitTime,
	})
	if err != nil {
		return ReceiveMessagesResult{}, err
	}

	return ReceiveMessagesResult{Messages: messages}, nil
}

// DeleteMessage removes a message from the queue using its receipt handle.
func (s *SqsServiceImpl) DeleteMessage(ctx context.Context, input DeleteMessageInput) error {
	queueURL := strings.TrimSpace(input.QueueURL)
	if queueURL == "" {
		return errors.New("queue url is required")
	}

	receiptHandle := strings.TrimSpace(input.ReceiptHandle)
	if receiptHandle == "" {
		return errors.New("receipt handle is required")
	}

	return s.repo.DeleteMessage(ctx, DeleteMessageRepositoryInput{
		QueueURL:      queueURL,
		ReceiptHandle: receiptHandle,
	})
}
