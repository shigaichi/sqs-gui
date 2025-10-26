package internal

import (
	"context"
	"encoding/base64"
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

type sqsAPI interface {
	ListQueues(ctx context.Context, params *sqs.ListQueuesInput, optFns ...func(*sqs.Options)) (*sqs.ListQueuesOutput, error)
	GetQueueAttributes(ctx context.Context, params *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) (*sqs.GetQueueAttributesOutput, error)
	CreateQueue(ctx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
	ListQueueTags(ctx context.Context, params *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) (*sqs.ListQueueTagsOutput, error)
	DeleteQueue(ctx context.Context, params *sqs.DeleteQueueInput, optFns ...func(*sqs.Options)) (*sqs.DeleteQueueOutput, error)
	PurgeQueue(ctx context.Context, params *sqs.PurgeQueueInput, optFns ...func(*sqs.Options)) (*sqs.PurgeQueueOutput, error)
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

// SqsRepository centralises access to SQS APIs.
type SqsRepository interface {
	ListQueues(ctx context.Context) ([]QueueSummary, error)
	CreateQueue(ctx context.Context, input CreateQueueRepositoryInput) (string, error)
	GetQueueDetail(ctx context.Context, queueURL string) (QueueDetail, error)
	DeleteQueue(ctx context.Context, queueURL string) error
	PurgeQueue(ctx context.Context, queueURL string) error
	SendMessage(ctx context.Context, input SendMessageRepositoryInput) error
	ReceiveMessages(ctx context.Context, input ReceiveMessagesRepositoryInput) ([]ReceivedMessage, error)
	DeleteMessage(ctx context.Context, input DeleteMessageRepositoryInput) error
}

// SqsRepositoryImpl uses the AWS SDK to talk to SQS.
type SqsRepositoryImpl struct {
	sqsClient sqsAPI
}

// CreateQueueRepositoryInput holds attributes for CreateQueue.
type CreateQueueRepositoryInput struct {
	Name       string
	Attributes map[string]string
}

type SendMessageRepositoryInput struct {
	QueueURL               string
	Body                   string
	MessageGroupID         string
	MessageDeduplicationID string
	DelaySeconds           *int32
	Attributes             map[string]string
}

// ReceiveMessagesRepositoryInput governs how ReceiveMessage API is called.
type ReceiveMessagesRepositoryInput struct {
	QueueURL        string
	MaxMessages     int32
	WaitTimeSeconds int32
}

// DeleteMessageRepositoryInput carries the data required to issue a DeleteMessage call.
type DeleteMessageRepositoryInput struct {
	QueueURL      string
	ReceiptHandle string
}

// NewSqsRepository constructs a repository instance.
func NewSqsRepository(c sqsAPI) SqsRepository {
	return &SqsRepositoryImpl{sqsClient: c}
}

// ListQueues fetches available queues.
func (s *SqsRepositoryImpl) ListQueues(ctx context.Context) ([]QueueSummary, error) {
	input := &sqs.ListQueuesInput{}
	baseAttributeNames := []types.QueueAttributeName{
		types.QueueAttributeNameCreatedTimestamp,
		types.QueueAttributeNameApproximateNumberOfMessages,
		types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
		types.QueueAttributeNameKmsMasterKeyId,
	}

	queues := make([]QueueSummary, 0)

	for {
		resp, err := s.sqsClient.ListQueues(ctx, input)
		if err != nil {
			return nil, errors.Wrap(err, "failed to call ListQueues API")
		}

		for _, url := range resp.QueueUrls {
			isFIFO := strings.HasSuffix(url, ".fifo")
			attributeNames := make([]types.QueueAttributeName, len(baseAttributeNames), len(baseAttributeNames)+2)
			copy(attributeNames, baseAttributeNames)
			if isFIFO {
				attributeNames = append(attributeNames, types.QueueAttributeNameFifoQueue, types.QueueAttributeNameContentBasedDeduplication)
			}

			attrs, err := s.sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
				QueueUrl:       aws.String(url),
				AttributeNames: attributeNames,
			})
			if err != nil {
				slog.Warn("failed to retrieve queue attributes", slog.String("queue_url", url), slog.Any("error", err))
				continue
			}

			attrMap := make(map[string]string, len(attrs.Attributes)+2)
			for key, value := range attrs.Attributes {
				attrMap[key] = value
			}

			if isFIFO {
				attrMap[string(types.QueueAttributeNameFifoQueue)] = "true"
			}

			queues = append(queues, buildQueueSummary(url, attrMap))
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

// DeleteQueue deletes the specified queue.
func (s *SqsRepositoryImpl) DeleteQueue(ctx context.Context, queueURL string) error {
	_, err := s.sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{QueueUrl: aws.String(queueURL)})
	if err != nil {
		return errors.Wrap(err, "failed to call DeleteQueue API")
	}

	return nil
}

// PurgeQueue removes all messages from the specified queue.
func (s *SqsRepositoryImpl) PurgeQueue(ctx context.Context, queueURL string) error {
	_, err := s.sqsClient.PurgeQueue(ctx, &sqs.PurgeQueueInput{QueueUrl: aws.String(queueURL)})
	if err != nil {
		return errors.Wrap(err, "failed to call PurgeQueue API")
	}

	return nil
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

// SendMessage enqueues a message into the specified queue.
func (s *SqsRepositoryImpl) SendMessage(ctx context.Context, input SendMessageRepositoryInput) error {
	req := &sqs.SendMessageInput{
		QueueUrl:    aws.String(input.QueueURL),
		MessageBody: aws.String(input.Body),
	}

	if input.DelaySeconds != nil {
		req.DelaySeconds = *input.DelaySeconds
	}

	messageGroupID := strings.TrimSpace(input.MessageGroupID)
	if messageGroupID != "" {
		req.MessageGroupId = aws.String(messageGroupID)
	}

	messageDeduplicationID := strings.TrimSpace(input.MessageDeduplicationID)
	if messageDeduplicationID != "" {
		req.MessageDeduplicationId = aws.String(messageDeduplicationID)
	}

	if len(input.Attributes) > 0 {
		req.MessageAttributes = make(map[string]types.MessageAttributeValue, len(input.Attributes))
		for key, value := range input.Attributes {
			if strings.TrimSpace(key) == "" {
				continue
			}
			req.MessageAttributes[key] = types.MessageAttributeValue{
				DataType:    aws.String("String"),
				StringValue: aws.String(value),
			}
		}
	}

	if _, err := s.sqsClient.SendMessage(ctx, req); err != nil {
		return errors.Wrap(err, "failed to call SendMessage API")
	}

	return nil
}

// ReceiveMessages fetches messages from the specified queue using ReceiveMessage.
func (s *SqsRepositoryImpl) ReceiveMessages(ctx context.Context, input ReceiveMessagesRepositoryInput) ([]ReceivedMessage, error) {
	req := &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(input.QueueURL),
		MaxNumberOfMessages:   input.MaxMessages,
		WaitTimeSeconds:       input.WaitTimeSeconds,
		VisibilityTimeout:     0,
		MessageAttributeNames: []string{"All"},
		MessageSystemAttributeNames: []types.MessageSystemAttributeName{
			types.MessageSystemAttributeNameApproximateReceiveCount,
			types.MessageSystemAttributeNameSentTimestamp,
			types.MessageSystemAttributeNameMessageGroupId,
			types.MessageSystemAttributeNameMessageDeduplicationId,
			types.MessageSystemAttributeNameSequenceNumber,
		},
	}

	resp, err := s.sqsClient.ReceiveMessage(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to call ReceiveMessage API")
	}

	messages := make([]ReceivedMessage, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		receiveCount := int32(0)
		if raw, ok := msg.Attributes[string(types.MessageSystemAttributeNameApproximateReceiveCount)]; ok {
			if value, err := strconv.ParseInt(raw, 10, 32); err == nil {
				receiveCount = int32(value)
			}
		}

		customKeys := make([]string, 0, len(msg.MessageAttributes))
		for key := range msg.MessageAttributes {
			customKeys = append(customKeys, key)
		}
		sort.Strings(customKeys)

		attributes := make([]MessageAttribute, 0, len(msg.MessageAttributes)+len(msg.Attributes))
		for _, key := range customKeys {
			value := msg.MessageAttributes[key]
			if value.StringValue != nil {
				attributes = append(attributes, MessageAttribute{Name: key, Value: aws.ToString(value.StringValue)})
				continue
			}
			if len(value.StringListValues) > 0 {
				attributes = append(attributes, MessageAttribute{Name: key, Value: strings.Join(value.StringListValues, ", ")})
				continue
			}
			if len(value.BinaryValue) > 0 {
				attributes = append(attributes, MessageAttribute{Name: key, Value: base64.StdEncoding.EncodeToString(value.BinaryValue)})
				continue
			}
			if len(value.BinaryListValues) > 0 {
				encoded := make([]string, len(value.BinaryListValues))
				for i, b := range value.BinaryListValues {
					encoded[i] = base64.StdEncoding.EncodeToString(b)
				}
				attributes = append(attributes, MessageAttribute{Name: key, Value: strings.Join(encoded, ", ")})
			}
		}

		systemKeys := make([]string, 0, len(msg.Attributes))
		for key := range msg.Attributes {
			systemKeys = append(systemKeys, key)
		}
		sort.Strings(systemKeys)
		for _, key := range systemKeys {
			attributes = append(attributes, MessageAttribute{Name: key, Value: formatSystemAttribute(key, msg.Attributes[key])})
		}

		messageID := aws.ToString(msg.MessageId)
		body := aws.ToString(msg.Body)
		messages = append(messages, ReceivedMessage{
			ID:            messageID,
			Body:          body,
			ReceiptHandle: aws.ToString(msg.ReceiptHandle),
			ReceiveCount:  receiveCount,
			Attributes:    attributes,
		})
	}

	return messages, nil
}

// DeleteMessage removes a message from the queue using its receipt handle.
func (s *SqsRepositoryImpl) DeleteMessage(ctx context.Context, input DeleteMessageRepositoryInput) error {
	_, err := s.sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(input.QueueURL),
		ReceiptHandle: aws.String(input.ReceiptHandle),
	})
	if err != nil {
		return errors.Wrap(err, "failed to call DeleteMessage API")
	}

	return nil
}

func formatSystemAttribute(key, value string) string {
	switch key {
	case string(types.MessageSystemAttributeNameSentTimestamp),
		string(types.MessageSystemAttributeNameApproximateFirstReceiveTimestamp):
		if value == "" {
			return value
		}
		if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
			return time.UnixMilli(ts).UTC().Format(time.RFC3339)
		}
	}

	return value
}
