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
