package internal

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func int32Ptr(v int32) *int32 {
	return &v
}

func TestSqsServiceImpl_Queues(t *testing.T) {
	repo := NewMockSqsRepository(t)

	q1 := QueueSummary{
		URL:                       "http://localhost:9324/000000000000/queue1",
		Name:                      "queue1",
		Type:                      QueueTypeStandard,
		CreatedAt:                 time.Now(),
		MessagesAvailable:         5,
		MessagesInFlight:          1,
		Encryption:                "None",
		ContentBasedDeduplication: false,
	}

	q2 := QueueSummary{
		URL:                       "http://localhost:9324/000000000000/queue2.fifo",
		Name:                      "queue2.fifo",
		Type:                      QueueTypeFIFO,
		CreatedAt:                 time.Now(),
		MessagesAvailable:         2,
		MessagesInFlight:          0,
		Encryption:                "None",
		ContentBasedDeduplication: true,
	}

	expected := []QueueSummary{q1, q2}

	repo.EXPECT().
		ListQueues(mock.Anything).
		Return(expected, nil).
		Once()

	service := &SqsServiceImpl{repo: repo}

	result, err := service.Queues(context.Background())
	assert.NoError(t, err)
	assert.ElementsMatch(t, expected, result)
}

func TestSqsServiceImpl_CreateQueue(t *testing.T) {
	type args struct {
		ctx   context.Context
		input CreateQueueInput
	}

	tests := []struct {
		name       string
		args       args
		arrange    func(t *testing.T, repo *MockSqsRepository, args args)
		want       CreateQueueResult
		wantErr    string
		assertMock func(t *testing.T, repo *MockSqsRepository)
	}{
		{
			name: "creates standard queue with trimmed name and defaults",
			args: args{
				ctx: context.Background(),
				input: CreateQueueInput{
					Name: " orders ",
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					CreateQueue(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input CreateQueueRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, "orders", input.Name)
						assert.Empty(t, input.Attributes)
					}).
					Return("https://sqs.local/orders", nil).
					Once()
			},
			want: CreateQueueResult{QueueURL: "https://sqs.local/orders"},
		},
		{
			name: "creates fifo queue and appends suffix when missing",
			args: args{
				ctx: context.Background(),
				input: CreateQueueInput{
					Name: "payments",
					Type: QueueTypeFIFO,
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					CreateQueue(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input CreateQueueRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, "payments.fifo", input.Name)
						assert.Equal(t, map[string]string{"FifoQueue": "true"}, input.Attributes)
					}).
					Return("https://sqs.local/payments.fifo", nil).
					Once()
			},
			want: CreateQueueResult{QueueURL: "https://sqs.local/payments.fifo"},
		},
		{
			name: "infers fifo queue when name already suffixed",
			args: args{
				ctx: context.Background(),
				input: CreateQueueInput{
					Name: "audit.fifo",
					Type: QueueTypeStandard,
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					CreateQueue(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input CreateQueueRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, "audit.fifo", input.Name)
						assert.Equal(t, map[string]string{"FifoQueue": "true"}, input.Attributes)
					}).
					Return("https://sqs.local/audit.fifo", nil).
					Once()
			},
			want: CreateQueueResult{QueueURL: "https://sqs.local/audit.fifo"},
		},
		{
			name: "returns error when queue name is blank",
			args: args{
				ctx: context.Background(),
				input: CreateQueueInput{
					Name: "   ",
				},
			},
			wantErr: "queue name is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "CreateQueue", mock.Anything, mock.Anything)
			},
		},
		{
			name: "returns error when queue type is invalid",
			args: args{
				ctx: context.Background(),
				input: CreateQueueInput{
					Name: "orders",
					Type: QueueType("dead-letter"),
				},
			},
			wantErr: "invalid queue type",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "CreateQueue", mock.Anything, mock.Anything)
			},
		},
		{
			name: "returns error when content based deduplication requested on standard queue",
			args: args{
				ctx: context.Background(),
				input: CreateQueueInput{
					Name:                      "orders",
					Type:                      QueueTypeStandard,
					ContentBasedDeduplication: true,
				},
			},
			wantErr: "content-based deduplication is only available for FIFO queues",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "CreateQueue", mock.Anything, mock.Anything)
			},
		},
		{
			name: "populates optional attributes for fifo queue",
			args: args{
				ctx: context.Background(),
				input: CreateQueueInput{
					Name:                      "events",
					Type:                      QueueTypeFIFO,
					DelaySeconds:              int32Ptr(10),
					MessageRetentionPeriod:    int32Ptr(3600),
					VisibilityTimeout:         int32Ptr(45),
					ContentBasedDeduplication: true,
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					CreateQueue(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input CreateQueueRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, "events.fifo", input.Name)
						assert.Equal(t, map[string]string{
							"ContentBasedDeduplication": "true",
							"DelaySeconds":              "10",
							"FifoQueue":                 "true",
							"MessageRetentionPeriod":    "3600",
							"VisibilityTimeout":         "45",
						}, input.Attributes)
					}).
					Return("https://sqs.local/events.fifo", nil).
					Once()
			},
			want: CreateQueueResult{QueueURL: "https://sqs.local/events.fifo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockSqsRepository(t)
			if tt.arrange != nil {
				tt.arrange(t, repo, tt.args)
			}

			service := &SqsServiceImpl{repo: repo}

			got, err := service.CreateQueue(tt.args.ctx, tt.args.input)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Equal(t, CreateQueueResult{}, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			if tt.assertMock != nil {
				tt.assertMock(t, repo)
			}
		})
	}
}

func TestSqsServiceImpl_QueueDetail(t *testing.T) {
	type args struct {
		ctx      context.Context
		queueURL string
	}

	tests := []struct {
		name       string
		args       args
		arrange    func(t *testing.T, repo *MockSqsRepository, args args)
		want       QueueDetail
		wantErr    string
		assertMock func(t *testing.T, repo *MockSqsRepository)
	}{
		{
			name: "returns queue detail when url provided",
			args: args{
				ctx:      context.Background(),
				queueURL: "https://sqs.local/orders",
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				detail := QueueDetail{
					QueueSummary: QueueSummary{
						URL:  args.queueURL,
						Name: "orders",
						Type: QueueTypeStandard,
					},
					Arn:            "arn:aws:sqs:local:000000000000:orders",
					LastModifiedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
					Attributes:     map[string]string{"VisibilityTimeout": "30"},
					Tags:           map[string]string{"env": "dev"},
				}

				repo.EXPECT().
					GetQueueDetail(mock.Anything, args.queueURL).
					Return(detail, nil).
					Once()
			},
			want: QueueDetail{
				QueueSummary: QueueSummary{
					URL:  "https://sqs.local/orders",
					Name: "orders",
					Type: QueueTypeStandard,
				},
				Arn:            "arn:aws:sqs:local:000000000000:orders",
				LastModifiedAt: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
				Attributes:     map[string]string{"VisibilityTimeout": "30"},
				Tags:           map[string]string{"env": "dev"},
			},
		},
		{
			name: "returns error when queue url is blank",
			args: args{
				ctx:      context.Background(),
				queueURL: "",
			},
			wantErr: "queue url is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "GetQueueDetail", mock.Anything, mock.Anything)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockSqsRepository(t)
			if tt.arrange != nil {
				tt.arrange(t, repo, tt.args)
			}

			service := &SqsServiceImpl{repo: repo}

			got, err := service.QueueDetail(tt.args.ctx, tt.args.queueURL)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Equal(t, QueueDetail{}, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			if tt.assertMock != nil {
				tt.assertMock(t, repo)
			}
		})
	}
}

func TestSqsServiceImpl_DeleteQueue(t *testing.T) {
	type args struct {
		ctx      context.Context
		queueURL string
	}

	tests := []struct {
		name       string
		args       args
		arrange    func(t *testing.T, repo *MockSqsRepository, args args)
		wantErr    string
		assertMock func(t *testing.T, repo *MockSqsRepository)
	}{
		{
			name: "deletes queue when url provided",
			args: args{
				ctx:      context.Background(),
				queueURL: "https://sqs.local/orders",
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					DeleteQueue(mock.Anything, args.queueURL).
					Return(nil).
					Once()
			},
		},
		{
			name: "returns error when queue url is blank",
			args: args{
				ctx:      context.Background(),
				queueURL: "",
			},
			wantErr: "queue url is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "DeleteQueue", mock.Anything, mock.Anything)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockSqsRepository(t)
			if tt.arrange != nil {
				tt.arrange(t, repo, tt.args)
			}

			service := &SqsServiceImpl{repo: repo}

			err := service.DeleteQueue(tt.args.ctx, tt.args.queueURL)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			if tt.assertMock != nil {
				tt.assertMock(t, repo)
			}
		})
	}
}

func TestSqsServiceImpl_PurgeQueue(t *testing.T) {
	type args struct {
		ctx      context.Context
		queueURL string
	}

	tests := []struct {
		name       string
		args       args
		arrange    func(t *testing.T, repo *MockSqsRepository, args args)
		wantErr    string
		assertMock func(t *testing.T, repo *MockSqsRepository)
	}{
		{
			name: "returns nil when queue url provided",
			args: args{
				ctx:      context.Background(),
				queueURL: "http://localhost:9324/000000000000/queue1",
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					PurgeQueue(mock.Anything, args.queueURL).
					Return(nil).
					Once()
			},
		},
		{
			name: "returns error when queue url is empty",
			args: args{
				ctx:      context.Background(),
				queueURL: "",
			},
			wantErr: "queue url is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "PurgeQueue", mock.Anything, mock.Anything)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockSqsRepository(t)
			if tt.arrange != nil {
				tt.arrange(t, repo, tt.args)
			}

			service := &SqsServiceImpl{repo: repo}

			err := service.PurgeQueue(tt.args.ctx, tt.args.queueURL)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			if tt.assertMock != nil {
				tt.assertMock(t, repo)
			}
		})
	}
}

func TestSqsServiceImpl_SendMessage(t *testing.T) {
	type args struct {
		ctx   context.Context
		input SendMessageInput
	}

	tests := []struct {
		name       string
		args       args
		arrange    func(t *testing.T, repo *MockSqsRepository, args args)
		wantErr    string
		assertMock func(t *testing.T, repo *MockSqsRepository)
	}{
		{
			name: "sends message with trimmed inputs and filtered attributes",
			args: args{
				ctx: context.Background(),
				input: SendMessageInput{
					QueueURL:       " https://sqs.local/queue ",
					Body:           "event",
					MessageGroupID: " group ",
					DelaySeconds:   int32Ptr(10),
					Attributes: []MessageAttribute{
						{Name: " TraceId ", Value: "123"},
						{Name: "", Value: "ignored"},
					},
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					SendMessage(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input SendMessageRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, "https://sqs.local/queue", input.QueueURL)
						assert.Equal(t, "event", input.Body)
						assert.Equal(t, "group", input.MessageGroupID)
						if assert.NotNil(t, input.DelaySeconds) {
							assert.Equal(t, int32(10), *input.DelaySeconds)
						}
						assert.Equal(t, map[string]string{"TraceId": "123"}, input.Attributes)
					}).
					Return(nil).
					Once()
			},
		},
		{
			name: "returns error when queue url is blank",
			args: args{
				ctx: context.Background(),
				input: SendMessageInput{
					QueueURL: "",
					Body:     "event",
				},
			},
			wantErr: "queue url is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "SendMessage", mock.Anything, mock.Anything)
			},
		},
		{
			name: "returns error when message body is blank",
			args: args{
				ctx: context.Background(),
				input: SendMessageInput{
					QueueURL: "https://sqs.local/queue",
					Body:     " ",
				},
			},
			wantErr: "message body is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "SendMessage", mock.Anything, mock.Anything)
			},
		},
		{
			name: "returns error when delay seconds below range",
			args: args{
				ctx: context.Background(),
				input: SendMessageInput{
					QueueURL:     "https://sqs.local/queue",
					Body:         "event",
					DelaySeconds: int32Ptr(-1),
				},
			},
			wantErr: "delay seconds must be between 0 and 900",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "SendMessage", mock.Anything, mock.Anything)
			},
		},
		{
			name: "returns error when delay seconds above range",
			args: args{
				ctx: context.Background(),
				input: SendMessageInput{
					QueueURL:     "https://sqs.local/queue",
					Body:         "event",
					DelaySeconds: int32Ptr(901),
				},
			},
			wantErr: "delay seconds must be between 0 and 900",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "SendMessage", mock.Anything, mock.Anything)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockSqsRepository(t)
			if tt.arrange != nil {
				tt.arrange(t, repo, tt.args)
			}

			service := &SqsServiceImpl{repo: repo}

			err := service.SendMessage(tt.args.ctx, tt.args.input)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			if tt.assertMock != nil {
				tt.assertMock(t, repo)
			}
		})
	}
}

func TestSqsServiceImpl_ReceiveMessages(t *testing.T) {
	type args struct {
		ctx   context.Context
		input ReceiveMessagesInput
	}

	tests := []struct {
		name       string
		args       args
		arrange    func(t *testing.T, repo *MockSqsRepository, args args)
		want       ReceiveMessagesResult
		wantErr    string
		assertMock func(t *testing.T, repo *MockSqsRepository)
	}{
		{
			name: "applies defaults when values not provided",
			args: args{
				ctx: context.Background(),
				input: ReceiveMessagesInput{
					QueueURL: " https://sqs.local/queue ",
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					ReceiveMessages(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input ReceiveMessagesRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, "https://sqs.local/queue", input.QueueURL)
						assert.Equal(t, int32(10), input.MaxMessages)
						assert.Equal(t, int32(20), input.WaitTimeSeconds)
					}).
					Return([]ReceivedMessage{{ID: "1", Body: "event"}}, nil).
					Once()
			},
			want: ReceiveMessagesResult{Messages: []ReceivedMessage{{ID: "1", Body: "event"}}},
		},
		{
			name: "clamps provided values below minimum",
			args: args{
				ctx: context.Background(),
				input: ReceiveMessagesInput{
					QueueURL:            "https://sqs.local/queue",
					MaxMessages:         0,
					WaitTimeSeconds:     -5,
					MaxMessagesProvided: true,
					WaitTimeProvided:    true,
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					ReceiveMessages(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input ReceiveMessagesRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, args.input.QueueURL, input.QueueURL)
						assert.Equal(t, int32(1), input.MaxMessages)
						assert.Equal(t, int32(0), input.WaitTimeSeconds)
					}).
					Return([]ReceivedMessage{}, nil).
					Once()
			},
			want: ReceiveMessagesResult{Messages: []ReceivedMessage{}},
		},
		{
			name: "clamps provided values above maximum",
			args: args{
				ctx: context.Background(),
				input: ReceiveMessagesInput{
					QueueURL:            "https://sqs.local/queue",
					MaxMessages:         25,
					WaitTimeSeconds:     40,
					MaxMessagesProvided: true,
					WaitTimeProvided:    true,
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					ReceiveMessages(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input ReceiveMessagesRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, args.input.QueueURL, input.QueueURL)
						assert.Equal(t, int32(10), input.MaxMessages)
						assert.Equal(t, int32(20), input.WaitTimeSeconds)
					}).
					Return([]ReceivedMessage{{ID: "1"}}, nil).
					Once()
			},
			want: ReceiveMessagesResult{Messages: []ReceivedMessage{{ID: "1"}}},
		},
		{
			name: "returns error when queue url is blank",
			args: args{
				ctx: context.Background(),
				input: ReceiveMessagesInput{
					QueueURL: " ",
				},
			},
			wantErr: "queue url is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "ReceiveMessages", mock.Anything, mock.Anything)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockSqsRepository(t)
			if tt.arrange != nil {
				tt.arrange(t, repo, tt.args)
			}

			service := &SqsServiceImpl{repo: repo}

			got, err := service.ReceiveMessages(tt.args.ctx, tt.args.input)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Equal(t, ReceiveMessagesResult{}, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			if tt.assertMock != nil {
				tt.assertMock(t, repo)
			}
		})
	}
}

func TestSqsServiceImpl_DeleteMessage(t *testing.T) {
	type args struct {
		ctx   context.Context
		input DeleteMessageInput
	}

	tests := []struct {
		name       string
		args       args
		arrange    func(t *testing.T, repo *MockSqsRepository, args args)
		wantErr    string
		assertMock func(t *testing.T, repo *MockSqsRepository)
	}{
		{
			name: "deletes message with trimmed inputs",
			args: args{
				ctx: context.Background(),
				input: DeleteMessageInput{
					QueueURL:      " https://sqs.local/queue ",
					ReceiptHandle: " receipt ",
				},
			},
			arrange: func(t *testing.T, repo *MockSqsRepository, args args) {
				repo.EXPECT().
					DeleteMessage(mock.Anything, mock.Anything).
					Run(func(ctx context.Context, input DeleteMessageRepositoryInput) {
						assert.Equal(t, args.ctx, ctx)
						assert.Equal(t, "https://sqs.local/queue", input.QueueURL)
						assert.Equal(t, "receipt", input.ReceiptHandle)
					}).
					Return(nil).
					Once()
			},
		},
		{
			name: "returns error when queue url is blank",
			args: args{
				ctx: context.Background(),
				input: DeleteMessageInput{
					QueueURL:      "",
					ReceiptHandle: "receipt",
				},
			},
			wantErr: "queue url is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "DeleteMessage", mock.Anything, mock.Anything)
			},
		},
		{
			name: "returns error when receipt handle is blank",
			args: args{
				ctx: context.Background(),
				input: DeleteMessageInput{
					QueueURL:      "https://sqs.local/queue",
					ReceiptHandle: " ",
				},
			},
			wantErr: "receipt handle is required",
			assertMock: func(t *testing.T, repo *MockSqsRepository) {
				repo.AssertNotCalled(t, "DeleteMessage", mock.Anything, mock.Anything)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := NewMockSqsRepository(t)
			if tt.arrange != nil {
				tt.arrange(t, repo, tt.args)
			}

			service := &SqsServiceImpl{repo: repo}

			err := service.DeleteMessage(tt.args.ctx, tt.args.input)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}

			if tt.assertMock != nil {
				tt.assertMock(t, repo)
			}
		})
	}
}
