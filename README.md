# go-job-runner

go-job-runner is a lightweight job processing system in Go that provides async execution, concurrency-safe state tracking, and graceful shutdown—designed to explore reliability patterns like retries and idempotency.

---

## Project Goal

Design and implement a job processing system that safely executes asynchronous tasks under concurrency, models failure scenarios, and prepares for scalability and resilience.

---

## MVP Scope

### In scope

- Post job
- Read job
- Worker pool
- State Machine
- Graceful Shutdown

### Out of scope

- DB persistence
- retry / backoff
- idempotency  key
- distributed multi-node
- metrics system
- rate limiting
- auth
- UI

---

## API Contract (Draft)

- POST /jobs returns job_id
- GET /jobs/{job_id} returns state

---

## Job State Machine

PENDING -> RUNNING -> (SUCCEEDED | FAILED | CANCELED)
