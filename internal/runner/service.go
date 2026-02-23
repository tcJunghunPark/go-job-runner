package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var ErrStopped = errors.New("runner is stopped")
var ErrJobNotFound = errors.New("job not found")

type Processor func(ctx context.Context, job Job) (string, error)

type jobRecord struct {
	job       Job
	state     State
	result    string
	err       string
	cancel    context.CancelFunc
	updatedAt time.Time
}

type Service struct {
	mu        sync.RWMutex
	jobs      map[string]*jobRecord
	queue     chan string
	wg        sync.WaitGroup
	processor Processor

	counter   uint64
	accepting bool
	baseCtx   context.Context
	cancelAll context.CancelFunc
}

func NewService(queueSize int, processor Processor) *Service {
	if queueSize <= 0 {
		queueSize = 64
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Service{
		jobs:      make(map[string]*jobRecord),
		queue:     make(chan string, queueSize),
		processor: processor,
		accepting: true,
		baseCtx:   ctx,
		cancelAll: cancel,
	}
}

func (s *Service) Start(nWorkers int) {
	if nWorkers <= 0 {
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
			_ = s.runJob(jobID) // TODO: process job
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
			ID:        id,
			Params:    params,
			CreatedAt: now,
		},
		state:     StatePending,
		updatedAt: now,
	}
	s.mu.Unlock()

	select {
	case <-ctx.Done():
		s.cancelJobProcess(id)
		return "", ctx.Err()
	case <-s.baseCtx.Done():
		s.cancelJobProcess(id)
		return "", ErrStopped
	case s.queue <- id:
		return id, nil
	}
}

func (s *Service) cancelJobProcess(jobID string) {
	s.mu.Lock()
	job := s.jobs[jobID]
	if job == nil {
		s.mu.Unlock()
		return
	}
	job.state = StateCanceled
	job.updatedAt = time.Now().UTC()
	s.mu.Unlock()
}

func (s *Service) getJobLocked(jobID string) (*jobRecord, error) {
	job := s.jobs[jobID]
	if job == nil {
		return nil, ErrJobNotFound
	}
	jobSnapshot := &jobRecord{
		job:       job.job,
		state:     job.state,
		result:    job.result,
		err:       job.err,
		updatedAt: job.updatedAt,
	}
	return jobSnapshot, nil
}

func (s *Service) runJob(jobID string) error {
	s.mu.Lock()
	jobSnapshot, err := s.getJobLocked(jobID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if jobSnapshot.state != StatePending {
		s.mu.Unlock()
		return nil
	}
	job := s.jobs[jobID]
	job.state = StateRunning
	job.updatedAt = time.Now().UTC()
	_, cancel := context.WithCancel(s.baseCtx)
	job.cancel = cancel
	s.mu.Unlock()
	// TODO process job
	s.mu.Lock()
	job = s.jobs[jobID]
	job.state = StateSucceeded
	job.cancel = nil
	job.updatedAt = time.Now().UTC()
	s.mu.Unlock()
	return nil
}
