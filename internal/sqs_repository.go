package internal

import (
	"context"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/cockroachdb/errors"
)

// SqsRepository centralises access to SQS APIs.
type SqsRepository interface {
	ListQueues(ctx context.Context) ([]QueueSummary, error)
	CreateQueue(ctx context.Context, input CreateQueueRepositoryInput) (string, error)
	GetQueueDetail(ctx context.Context, queueURL string) (QueueDetail, error)
}

// SqsRepositoryImpl uses the AWS SDK to talk to SQS.
type SqsRepositoryImpl struct {
	sqsClient *sqs.Client
}

// CreateQueueRepositoryInput holds attributes for CreateQueue.
type CreateQueueRepositoryInput struct {
	Name       string
	Attributes map[string]string
}

// NewSqsRepository constructs a repository instance.
func NewSqsRepository(c *sqs.Client) SqsRepository {
	return &SqsRepositoryImpl{sqsClient: c}
}

// ListQueues fetches available queues.
func (s *SqsRepositoryImpl) ListQueues(ctx context.Context) ([]QueueSummary, error) {
	input := &sqs.ListQueuesInput{}
	attributeNames := []types.QueueAttributeName{
		types.QueueAttributeNameCreatedTimestamp,
		types.QueueAttributeNameApproximateNumberOfMessages,
		types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
		types.QueueAttributeNameContentBasedDeduplication,
		types.QueueAttributeNameKmsMasterKeyId,
		types.QueueAttributeNameFifoQueue,
	}

	queues := make([]QueueSummary, 0)

	for {
		resp, err := s.sqsClient.ListQueues(ctx, input)
		if err != nil {
			return nil, errors.Wrap(err, "failed to call ListQueues API")
		}

		for _, url := range resp.QueueUrls {
			attrs, err := s.sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
				QueueUrl:       aws.String(url),
				AttributeNames: attributeNames,
			})
			if err != nil {
				slog.Warn("failed to retrieve queue attributes", slog.String("queue_url", url), slog.Any("error", err))
				continue
			}

			queues = append(queues, buildQueueSummary(url, attrs.Attributes))
		}

		if resp.NextToken == nil {
			break
		}
		input.NextToken = resp.NextToken
	}

	sort.Slice(queues, func(i, j int) bool {
		return queues[i].Name < queues[j].Name
	})

	return queues, nil
}

// CreateQueue creates a new queue.
func (s *SqsRepositoryImpl) CreateQueue(ctx context.Context, input CreateQueueRepositoryInput) (string, error) {
	resp, err := s.sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  aws.String(input.Name),
		Attributes: input.Attributes,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to call CreateQueue API")
	}
	if resp.QueueUrl == nil {
		return "", errors.New("CreateQueue API response does not contain QueueUrl")
	}

	return *resp.QueueUrl, nil
}

// GetQueueDetail retrieves full queue information, including attributes and tags.
func (s *SqsRepositoryImpl) GetQueueDetail(ctx context.Context, queueURL string) (QueueDetail, error) {
	resp, err := s.sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(queueURL),
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameAll},
	})
	if err != nil {
		return QueueDetail{}, errors.Wrap(err, "failed to call GetQueueAttributes API")
	}

	attributes := make(map[string]string, len(resp.Attributes))
	for key, value := range resp.Attributes {
		attributes[key] = value
	}

	summary := buildQueueSummary(queueURL, attributes)
	lastModified := parseUnixTime(attributes[string(types.QueueAttributeNameLastModifiedTimestamp)])
	arn := attributes[string(types.QueueAttributeNameQueueArn)]

	detail := QueueDetail{
		QueueSummary:   summary,
		Arn:            arn,
		LastModifiedAt: lastModified,
		Attributes:     attributes,
	}

	tagResp, err := s.sqsClient.ListQueueTags(ctx, &sqs.ListQueueTagsInput{QueueUrl: aws.String(queueURL)})
	if err != nil {
		slog.Warn("failed to retrieve queue tags", slog.String("queue_url", queueURL), slog.Any("error", err))
	} else {
		if len(tagResp.Tags) > 0 {
			tags := make(map[string]string, len(tagResp.Tags))
			for key, value := range tagResp.Tags {
				tags[key] = value
			}
			detail.Tags = tags
		}
	}

	return detail, nil
}

// buildQueueSummary normalises queue attributes for presentation.
func buildQueueSummary(queueURL string, attributes map[string]string) QueueSummary {
	name := queueURL
	if idx := strings.LastIndex(queueURL, "/"); idx >= 0 {
		name = queueURL[idx+1:]
	}

	createdAt := time.Time{}
	if raw, ok := attributes[string(types.QueueAttributeNameCreatedTimestamp)]; ok {
		if ts, err := strconv.ParseInt(raw, 10, 64); err == nil {
			createdAt = time.Unix(ts, 0).UTC()
		}
	}

	messagesAvailable := parseInt64(attributes[string(types.QueueAttributeNameApproximateNumberOfMessages)])
	messagesInFlight := parseInt64(attributes[string(types.QueueAttributeNameApproximateNumberOfMessagesNotVisible)])
	contentDedup := attributes[string(types.QueueAttributeNameContentBasedDeduplication)] == "true"
	kmsKey := attributes[string(types.QueueAttributeNameKmsMasterKeyId)]
	fifoFlag := attributes[string(types.QueueAttributeNameFifoQueue)] == "true"

	queueType := QueueTypeStandard
	if fifoFlag || strings.HasSuffix(name, ".fifo") {
		queueType = QueueTypeFIFO
	}

	encryption := "None"
	if kmsKey != "" {
		encryption = "KMS"
	}

	return QueueSummary{
		URL:                       queueURL,
		Name:                      name,
		Type:                      queueType,
		CreatedAt:                 createdAt,
		MessagesAvailable:         messagesAvailable,
		MessagesInFlight:          messagesInFlight,
		Encryption:                encryption,
		ContentBasedDeduplication: contentDedup,
	}
}

// parseInt64 converts optional numeric attributes safely.
func parseInt64(raw string) int64 {
	if raw == "" {
		return 0
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		slog.Debug("failed to parse integer", slog.String("value", raw), slog.Any("error", err))
		return 0
	}

	return value
}

// parseUnixTime converts seconds since epoch to time.Time.
func parseUnixTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}

	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		slog.Debug("failed to parse timestamp", slog.String("value", raw), slog.Any("error", err))
		return time.Time{}
	}

	return time.Unix(value, 0).UTC()
}
