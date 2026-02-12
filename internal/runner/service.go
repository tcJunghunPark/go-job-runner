package runner

import (
	"context"
	"sync"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

var ErrStopped = errors.New("Runner is stopped")

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

	counter uint64
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

func (s *Service) Start(nWorkers int) {
	if nWorkers <=0 {
		nWorkers = 1
	}

	for i := 0; i < nWorkers; i++ {
		s.wg.Add(1)
		go s.workerLoop()
	}
}

func (s *Service) workerLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.baseCtx.Done():
			return
		case jobID, ok := <-s.queue:
			if !ok {
				return
			}
			_ = jobID // TODO: process job
		}
	}
}

func (s *Service) Submit(ctx context.Context, params map[string]string) (string, error) {
	s.mu.Lock()
	if !s.accepting {
		s.mu.Unlock()
		return "", ErrStopped
	}

	now := time.Now().UTC()
	id := fmt.Sprintf("job-%d-%d", now.UnixNano(), atomic.AddUint64(&s.counter, 1))

	s.jobs[id] = &jobRecord{
		job: Job{
			ID: id,
			Params: params,
			CreatedAt: now,
		},
		state: StatePending,
		updatedAt: now.UnixNano(),
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <- s.baseCtx.Done():
		return "", ErrStopped
	case s.queue <- id:
		return id, nil
	}
}
