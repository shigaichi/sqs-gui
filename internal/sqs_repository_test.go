package internal

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSqsRepositoryImpl_ListQueues(t *testing.T) {
	ctx := context.Background()
	t.Run("returns sorted queues across pages and skips attribute failures", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		firstPage := &sqs.ListQueuesOutput{
			QueueUrls: []string{
				"https://sqs.local/000000000000/queue-z",
				"https://sqs.local/000000000000/queue-b",
			},
			NextToken: aws.String("next-token"),
		}

		secondPage := &sqs.ListQueuesOutput{
			QueueUrls: []string{
				"https://sqs.local/000000000000/queue-a.fifo",
			},
		}

		api.EXPECT().
			ListQueues(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.ListQueuesInput, optFns ...func(*sqs.Options)) {
				require.Equal(t, ctx, callCtx)
				assert.Nil(t, input.NextToken)
			}).
			Return(firstPage, nil).
			Once()

		api.EXPECT().
			GetQueueAttributes(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String("https://sqs.local/000000000000/queue-z"), input.QueueUrl)
				assert.ElementsMatch(t, []types.QueueAttributeName{
					types.QueueAttributeNameCreatedTimestamp,
					types.QueueAttributeNameApproximateNumberOfMessages,
					types.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
					types.QueueAttributeNameContentBasedDeduplication,
					types.QueueAttributeNameKmsMasterKeyId,
					types.QueueAttributeNameFifoQueue,
				}, input.AttributeNames)
			}).
			Return(&sqs.GetQueueAttributesOutput{
				Attributes: map[string]string{
					string(types.QueueAttributeNameCreatedTimestamp):                      "1700000000",
					string(types.QueueAttributeNameApproximateNumberOfMessages):           "5",
					string(types.QueueAttributeNameApproximateNumberOfMessagesNotVisible): "1",
					string(types.QueueAttributeNameContentBasedDeduplication):             "false",
					string(types.QueueAttributeNameKmsMasterKeyId):                        "",
					string(types.QueueAttributeNameFifoQueue):                             "false",
				},
				ResultMetadata: middleware.Metadata{},
			}, nil).
			Once()

		api.EXPECT().
			GetQueueAttributes(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String("https://sqs.local/000000000000/queue-b"), input.QueueUrl)
			}).
			Return(nil, errors.New("boom")).
			Once()

		api.EXPECT().
			ListQueues(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.ListQueuesInput, optFns ...func(*sqs.Options)) {
				require.Equal(t, ctx, callCtx)
				require.NotNil(t, input.NextToken)
				assert.Equal(t, "next-token", aws.ToString(input.NextToken))
			}).
			Return(secondPage, nil).
			Once()

		api.EXPECT().
			GetQueueAttributes(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String("https://sqs.local/000000000000/queue-a.fifo"), input.QueueUrl)
			}).
			Return(&sqs.GetQueueAttributesOutput{
				Attributes: map[string]string{
					string(types.QueueAttributeNameCreatedTimestamp):                      "1700001000",
					string(types.QueueAttributeNameApproximateNumberOfMessages):           "10",
					string(types.QueueAttributeNameApproximateNumberOfMessagesNotVisible): "0",
					string(types.QueueAttributeNameContentBasedDeduplication):             "true",
					string(types.QueueAttributeNameKmsMasterKeyId):                        "alias/kms",
					string(types.QueueAttributeNameFifoQueue):                             "true",
				},
				ResultMetadata: middleware.Metadata{},
			}, nil).
			Once()

		queues, err := repo.ListQueues(ctx)
		require.NoError(t, err)

		expected := []QueueSummary{
			{
				URL:                       "https://sqs.local/000000000000/queue-a.fifo",
				Name:                      "queue-a.fifo",
				Type:                      QueueTypeFIFO,
				CreatedAt:                 time.Unix(1700001000, 0).UTC(),
				MessagesAvailable:         10,
				MessagesInFlight:          0,
				Encryption:                "KMS",
				ContentBasedDeduplication: true,
			},
			{
				URL:                       "https://sqs.local/000000000000/queue-z",
				Name:                      "queue-z",
				Type:                      QueueTypeStandard,
				CreatedAt:                 time.Unix(1700000000, 0).UTC(),
				MessagesAvailable:         5,
				MessagesInFlight:          1,
				Encryption:                "None",
				ContentBasedDeduplication: false,
			},
		}

		assert.Equal(t, expected, queues)
	})

	t.Run("propagates list queues errors", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			ListQueues(mock.Anything, mock.Anything).
			Return(nil, errors.New("network")).
			Once()

		queues, err := repo.ListQueues(ctx)
		assert.Nil(t, queues)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to call ListQueues API")
	})
}

func TestSqsRepositoryImpl_CreateQueue(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name    string
		input   CreateQueueRepositoryInput
		arrange func(api *mocksqsAPI)
		want    string
		wantErr string
	}{
		{
			name: "returns queue url on success",
			input: CreateQueueRepositoryInput{
				Name: "orders",
				Attributes: map[string]string{
					"VisibilityTimeout": "30",
				},
			},
			arrange: func(api *mocksqsAPI) {
				api.EXPECT().
					CreateQueue(mock.Anything, mock.Anything).
					Run(func(callCtx context.Context, params *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) {
						assert.Equal(t, ctx, callCtx)
						require.NotNil(t, params.QueueName)
						assert.Equal(t, "orders", aws.ToString(params.QueueName))
						assert.Equal(t, map[string]string{"VisibilityTimeout": "30"}, params.Attributes)
					}).
					Return(&sqs.CreateQueueOutput{QueueUrl: aws.String("https://sqs.local/orders")}, nil).
					Once()
			},
			want: "https://sqs.local/orders",
		},
		{
			name:  "wraps api error",
			input: CreateQueueRepositoryInput{Name: "orders"},
			arrange: func(api *mocksqsAPI) {
				api.EXPECT().
					CreateQueue(mock.Anything, mock.Anything).
					Return(nil, errors.New("boom")).
					Once()
			},
			wantErr: "failed to call CreateQueue API",
		},
		{
			name:  "returns error when queue url is missing",
			input: CreateQueueRepositoryInput{Name: "orders"},
			arrange: func(api *mocksqsAPI) {
				api.EXPECT().
					CreateQueue(mock.Anything, mock.Anything).
					Return(&sqs.CreateQueueOutput{}, nil).
					Once()
			},
			wantErr: "CreateQueue API response does not contain QueueUrl",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			api := newMocksqsAPI(t)
			if tt.arrange != nil {
				tt.arrange(api)
			}

			repo := &SqsRepositoryImpl{sqsClient: api}

			got, err := repo.CreateQueue(ctx, tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Empty(t, got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSqsRepositoryImpl_GetQueueDetail(t *testing.T) {
	ctx := context.Background()
	queueURL := "https://sqs.local/000000000000/queue.fifo"

	t.Run("returns detail with attributes and tags", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		attrs := map[string]string{
			string(types.QueueAttributeNameCreatedTimestamp):                      "1700000000",
			string(types.QueueAttributeNameApproximateNumberOfMessages):           "3",
			string(types.QueueAttributeNameApproximateNumberOfMessagesNotVisible): "1",
			string(types.QueueAttributeNameContentBasedDeduplication):             "true",
			string(types.QueueAttributeNameKmsMasterKeyId):                        "alias/kms",
			string(types.QueueAttributeNameFifoQueue):                             "true",
			string(types.QueueAttributeNameLastModifiedTimestamp):                 "1700000500",
			string(types.QueueAttributeNameQueueArn):                              "arn:aws:sqs:region:acct:queue.fifo",
		}

		api.EXPECT().
			GetQueueAttributes(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.GetQueueAttributesInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String(queueURL), input.QueueUrl)
				assert.Equal(t, []types.QueueAttributeName{types.QueueAttributeNameAll}, input.AttributeNames)
			}).
			Return(&sqs.GetQueueAttributesOutput{Attributes: attrs}, nil).
			Once()

		api.EXPECT().
			ListQueueTags(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.ListQueueTagsInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String(queueURL), input.QueueUrl)
			}).
			Return(&sqs.ListQueueTagsOutput{Tags: map[string]string{"env": "dev", "team": "platform"}}, nil).
			Once()

		detail, err := repo.GetQueueDetail(ctx, queueURL)
		require.NoError(t, err)

		expectedSummary := QueueSummary{
			URL:                       queueURL,
			Name:                      "queue.fifo",
			Type:                      QueueTypeFIFO,
			CreatedAt:                 time.Unix(1700000000, 0).UTC(),
			MessagesAvailable:         3,
			MessagesInFlight:          1,
			Encryption:                "KMS",
			ContentBasedDeduplication: true,
		}

		expectedDetail := QueueDetail{
			QueueSummary:   expectedSummary,
			Arn:            "arn:aws:sqs:region:acct:queue.fifo",
			LastModifiedAt: time.Unix(1700000500, 0).UTC(),
			Attributes:     attrs,
			Tags:           map[string]string{"env": "dev", "team": "platform"},
		}

		assert.Equal(t, expectedDetail, detail)
	})

	t.Run("only attributes when listing tags fails", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		attrs := map[string]string{
			string(types.QueueAttributeNameCreatedTimestamp):          "1700000000",
			string(types.QueueAttributeNameQueueArn):                  "arn",
			string(types.QueueAttributeNameFifoQueue):                 "false",
			string(types.QueueAttributeNameKmsMasterKeyId):            "",
			string(types.QueueAttributeNameContentBasedDeduplication): "false",
		}

		api.EXPECT().
			GetQueueAttributes(mock.Anything, mock.Anything).
			Return(&sqs.GetQueueAttributesOutput{Attributes: attrs}, nil).
			Once()

		api.EXPECT().
			ListQueueTags(mock.Anything, mock.Anything).
			Return(nil, errors.New("timeout")).
			Once()

		detail, err := repo.GetQueueDetail(ctx, queueURL)
		require.NoError(t, err)
		assert.Nil(t, detail.Tags)
		assert.Equal(t, attrs, detail.Attributes)
	})

	t.Run("returns error when get attributes fails", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			GetQueueAttributes(mock.Anything, mock.Anything).
			Return(nil, errors.New("boom")).
			Once()

		detail, err := repo.GetQueueDetail(ctx, queueURL)
		assert.Empty(t, detail)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to call GetQueueAttributes API")
	})
}

func TestSqsRepositoryImpl_DeleteQueue(t *testing.T) {
	ctx := context.Background()
	queueURL := "https://sqs.local/orders"

	t.Run("calls delete queue", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			DeleteQueue(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.DeleteQueueInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String(queueURL), input.QueueUrl)
			}).
			Return(&sqs.DeleteQueueOutput{}, nil).
			Once()

		err := repo.DeleteQueue(ctx, queueURL)
		require.NoError(t, err)
	})

	t.Run("wraps api error", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			DeleteQueue(mock.Anything, mock.Anything).
			Return(nil, errors.New("boom")).
			Once()

		err := repo.DeleteQueue(ctx, queueURL)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to call DeleteQueue API")
	})
}

func TestSqsRepositoryImpl_PurgeQueue(t *testing.T) {
	ctx := context.Background()
	queueURL := "https://sqs.local/orders"

	t.Run("purges queue", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			PurgeQueue(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, input *sqs.PurgeQueueInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String(queueURL), input.QueueUrl)
			}).
			Return(&sqs.PurgeQueueOutput{}, nil).
			Once()

		err := repo.PurgeQueue(ctx, queueURL)
		require.NoError(t, err)
	})

	t.Run("wraps api error", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			PurgeQueue(mock.Anything, mock.Anything).
			Return(nil, errors.New("boom")).
			Once()

		err := repo.PurgeQueue(ctx, queueURL)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to call PurgeQueue API")
	})
}

func TestSqsRepositoryImpl_SendMessage(t *testing.T) {
	ctx := context.Background()

	t.Run("sends message with trimmed group id and attributes", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		delay := int32(5)
		input := SendMessageRepositoryInput{
			QueueURL:               "https://sqs.local/orders",
			Body:                   "hello",
			MessageGroupID:         " group-1 ",
			MessageDeduplicationID: " dedup-1 ",
			DelaySeconds:           &delay,
			Attributes: map[string]string{
				"orderId": "123",
				"ignored": "",
				"":        "skip",
			},
		}

		api.EXPECT().
			SendMessage(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String(input.QueueURL), params.QueueUrl)
				assert.Equal(t, aws.String("hello"), params.MessageBody)
				assert.Equal(t, int32(5), params.DelaySeconds)
				require.NotNil(t, params.MessageGroupId)
				assert.Equal(t, "group-1", aws.ToString(params.MessageGroupId))
				require.NotNil(t, params.MessageDeduplicationId)
				assert.Equal(t, "dedup-1", aws.ToString(params.MessageDeduplicationId))
				require.Len(t, params.MessageAttributes, 2)
				attr := params.MessageAttributes["orderId"]
				assert.Equal(t, aws.String("String"), attr.DataType)
				assert.Equal(t, aws.String("123"), attr.StringValue)
				_, hasBlank := params.MessageAttributes[""]
				assert.False(t, hasBlank)
			}).
			Return(&sqs.SendMessageOutput{}, nil).
			Once()

		err := repo.SendMessage(ctx, input)
		require.NoError(t, err)
	})

	t.Run("wraps api error", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			SendMessage(mock.Anything, mock.Anything).
			Return(nil, errors.New("boom")).
			Once()

		err := repo.SendMessage(ctx, SendMessageRepositoryInput{QueueURL: "https://sqs.local/orders", Body: "hello"})
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to call SendMessage API")
	})
}

func TestSqsRepositoryImpl_ReceiveMessages(t *testing.T) {
	ctx := context.Background()

	t.Run("converts message attributes and system attributes", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		input := ReceiveMessagesRepositoryInput{
			QueueURL:        "https://sqs.local/orders",
			MaxMessages:     5,
			WaitTimeSeconds: 10,
		}

		api.EXPECT().
			ReceiveMessage(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String(input.QueueURL), params.QueueUrl)
				assert.Equal(t, input.MaxMessages, params.MaxNumberOfMessages)
				assert.Equal(t, input.WaitTimeSeconds, params.WaitTimeSeconds)
				assert.Equal(t, []string{"All"}, params.MessageAttributeNames)
			}).
			Return(&sqs.ReceiveMessageOutput{
				Messages: []types.Message{
					{
						MessageId:     aws.String("msg-1"),
						ReceiptHandle: aws.String("receipt-1"),
						Body:          aws.String("hello"),
						Attributes: map[string]string{
							string(types.MessageSystemAttributeNameApproximateReceiveCount):          "2",
							string(types.MessageSystemAttributeNameApproximateFirstReceiveTimestamp): "1700001000000",
							string(types.MessageSystemAttributeNameMessageDeduplicationId):           "dedup-1",
							string(types.MessageSystemAttributeNameMessageGroupId):                   "group-1",
							string(types.MessageSystemAttributeNameSentTimestamp):                    "1700002000000",
						},
						MessageAttributes: map[string]types.MessageAttributeValue{
							"CustomBinary": {
								BinaryValue: []byte{0x01, 0x02},
							},
							"CustomBinaryList": {
								BinaryListValues: [][]byte{{0x03}, {0x04}},
							},
							"CustomList": {
								StringListValues: []string{"hello", "world"},
							},
							"CustomString": {
								StringValue: aws.String("value"),
							},
						},
					},
				},
			}, nil).
			Once()

		messages, err := repo.ReceiveMessages(ctx, input)
		require.NoError(t, err)

		expected := []ReceivedMessage{
			{
				ID:            "msg-1",
				Body:          "hello",
				ReceiptHandle: "receipt-1",
				ReceiveCount:  2,
				Attributes: []MessageAttribute{
					{Name: "CustomBinary", Value: base64.StdEncoding.EncodeToString([]byte{0x01, 0x02})},
					{Name: "CustomBinaryList", Value: base64.StdEncoding.EncodeToString([]byte{0x03}) + ", " + base64.StdEncoding.EncodeToString([]byte{0x04})},
					{Name: "CustomList", Value: "hello, world"},
					{Name: "CustomString", Value: "value"},
					{Name: string(types.MessageSystemAttributeNameApproximateFirstReceiveTimestamp), Value: time.UnixMilli(1700001000000).UTC().Format(time.RFC3339)},
					{Name: string(types.MessageSystemAttributeNameApproximateReceiveCount), Value: "2"},
					{Name: string(types.MessageSystemAttributeNameMessageDeduplicationId), Value: "dedup-1"},
					{Name: string(types.MessageSystemAttributeNameMessageGroupId), Value: "group-1"},
					{Name: string(types.MessageSystemAttributeNameSentTimestamp), Value: time.UnixMilli(1700002000000).UTC().Format(time.RFC3339)},
				},
			},
		}

		assert.Equal(t, expected, messages)
	})

	t.Run("wraps receive message errors", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			ReceiveMessage(mock.Anything, mock.Anything).
			Return(nil, errors.New("boom")).
			Once()

		messages, err := repo.ReceiveMessages(ctx, ReceiveMessagesRepositoryInput{QueueURL: "https://sqs.local/orders"})
		assert.Nil(t, messages)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to call ReceiveMessage API")
	})
}

func TestSqsRepositoryImpl_DeleteMessage(t *testing.T) {
	ctx := context.Background()
	input := DeleteMessageRepositoryInput{QueueURL: "https://sqs.local/orders", ReceiptHandle: "abc"}

	t.Run("deletes message", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			DeleteMessage(mock.Anything, mock.Anything).
			Run(func(callCtx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) {
				assert.Equal(t, ctx, callCtx)
				assert.Equal(t, aws.String(input.QueueURL), params.QueueUrl)
				assert.Equal(t, aws.String(input.ReceiptHandle), params.ReceiptHandle)
			}).
			Return(&sqs.DeleteMessageOutput{}, nil).
			Once()

		err := repo.DeleteMessage(ctx, input)
		require.NoError(t, err)
	})

	t.Run("wraps api error", func(t *testing.T) {
		api := newMocksqsAPI(t)
		repo := &SqsRepositoryImpl{sqsClient: api}

		api.EXPECT().
			DeleteMessage(mock.Anything, mock.Anything).
			Return(nil, errors.New("boom")).
			Once()

		err := repo.DeleteMessage(ctx, input)
		require.Error(t, err)
		assert.ErrorContains(t, err, "failed to call DeleteMessage API")
	})
}
