package internal

type SqsService interface {
	Queues()
	CreateQueue()
}

type SqsServiceImpl struct {
	s SqsRepository
}

func NewSqsService(s SqsRepository) SqsService {
	return &SqsServiceImpl{s: s}
}

func (s *SqsServiceImpl) Queues() {
	// TODO : 実装。引数、返り値を変えても良い
	s.s.ListQueues()
}

func (s *SqsServiceImpl) CreateQueue() {
	// TODO : 実装。引数、返り値を変えても良い
	_ = s.s.CreateQueue()
}
