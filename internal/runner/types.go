package runner

import "time"

type State string

const (
	StatePending   State = "PENDING"
	StateRunning   State = "RUNNING"
	StateSucceeded State = "SUCCEEDED"
	StateFailed    State = "FAILED"
	StateCanceled  State = "CANCELED"
)
type Job struct {
	ID    string
	Params map[string]string
	CreatedAt time.Time
}

type Snapshot struct {
	ID string
	Params map[string]string
	State State
	Result string
	Error string
	CreatedAt time.Time
	UpdatedAt time.Time
}
