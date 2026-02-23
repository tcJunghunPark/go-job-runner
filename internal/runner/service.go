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
			_ = s.runJob(jobID)
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

func (s *Service) getJobLocked(jobID string) (*Snapshot, error) {
	record := s.jobs[jobID]
	if record == nil {
		return nil, ErrJobNotFound
	}
	jobSnapshot := &Snapshot{
		ID:        jobID,
		Params:    cloneParams(record.job.Params),
		State:     record.state,
		Result:    record.result,
		Error:     record.err,
		CreatedAt: record.job.CreatedAt,
		UpdatedAt: record.updatedAt,
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
	if jobSnapshot.State != StatePending {
		s.mu.Unlock()
		return nil
	}
	job := s.jobs[jobID]
	job.state = StateRunning
	job.updatedAt = time.Now().UTC()
	_, cancel := context.WithCancel(s.baseCtx)
	job.cancel = cancel
	s.mu.Unlock()
	// TODO: Call job processor
	s.mu.Lock()
	job = s.jobs[jobID]

	// TODO: Update job state based on processing result
	job.cancel = nil
	job.updatedAt = time.Now().UTC()
	s.mu.Unlock()
	return nil
}

func cloneParams(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (s *Service) GetJob(id string) (*Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, exists := s.jobs[id]
	if !exists {
		return &Snapshot{}, ErrJobNotFound
	}
	return &Snapshot{
		ID:        id,
		Params:    cloneParams(job.job.Params),
		State:     job.state,
		Result:    job.result,
		Error:     job.err,
		CreatedAt: job.job.CreatedAt,
		UpdatedAt: job.updatedAt,
	}, nil
}

func (s *Service) CancelJob(id string) error {
	s.mu.Lock()
	job, exists := s.jobs[id]
	if !exists {
		s.mu.Unlock()
		return ErrJobNotFound
	}
	if job.state != StatePending && job.state != StateRunning {
		s.mu.Unlock()
		return nil
	}
	job.state = StateCanceled
	job.updatedAt = time.Now().UTC()
	cancel := job.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *Service) Stop() {
	s.mu.Lock()
	if s.accepting {
		s.accepting = false
	}
	close(s.queue)
	s.mu.Unlock()
	s.wg.Wait()
	s.cancelAll()
}
