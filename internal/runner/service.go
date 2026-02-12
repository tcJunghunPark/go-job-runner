pakage runner

import (
	"context"
	"sync"
)

type Processor func(ctx context.Context, job Job) (string, error)

type jobRecord struct {
	job Job
	state State
	result string
	err string
	cancel context.CancelFunc
	updatedAt int64
}

type Service struct {
	mu sync.RWMutex
	jobs map[string]*jobRecord
	queue chan string
	wg sync.WaitGroup
	processor Processor

	accepting bool
	baseCtx context.Context
	cancelAll context.CancelFunc
}

func NewService(queueSize int, processor Processor) *Service {
	if queueSize <= 0 {
		queueSize = 64
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		jobs: make(map[string]*jobRecord),
		queue: make(chan string, queueSize),
		processor: processor,
		accepting: true,
		baseCtx: ctx,
		cancelAll: cancel,
	}
}
