package internal

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

type SqsRepository interface {
	ListQueues()
	CreateQueue() error
}

type SqsRepositoryImpl struct {
	sqsClient sqs.Client
}

func NewSqsRepository(c sqs.Client) SqsRepository {
	return &SqsRepositoryImpl{sqsClient: c}
}

func (s *SqsRepositoryImpl) ListQueues() {
	// TODO : 実装。引数、返り値を変えても良い
	// SQSアクセス処理サンプル
	s.sqsClient.ListQueues(context.TODO(), &sqs.ListQueuesInput{})

}

func (s *SqsRepositoryImpl) CreateQueue() error {
	// TODO : 実装。引数、返り値を変えても良い
	return nil
}
