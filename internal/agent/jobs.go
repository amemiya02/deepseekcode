package agent

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobKind distinguishes between different background job types.
type JobKind int

const (
	JobSubagent JobKind = iota
	JobBackgroundBash
)

// String returns the human-readable name of the job kind.
func (k JobKind) String() string {
	switch k {
	case JobSubagent:
		return "subagent"
	case JobBackgroundBash:
		return "background_bash"
	default:
		return "unknown"
	}
}

// JobState represents the current state of a background job.
type JobState int

const (
	JobRunning JobState = iota
	JobSucceeded
	JobFailed
	JobCanceled
)

// String returns the human-readable name of the job state.
func (s JobState) String() string {
	switch s {
	case JobRunning:
		return "running"
	case JobSucceeded:
		return "succeeded"
	case JobFailed:
		return "failed"
	case JobCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// Job represents a running or completed background job.
type Job struct {
	ID          string
	Kind        JobKind
	Description string
	StartedAt   time.Time
	FinishedAt  time.Time
	State       JobState
	Summary     string

	cancel       context.CancelFunc
	mu           sync.Mutex
	out          []byte
	droppedBytes int64
	maxBytes     int
}

// AppendOutput appends data to the job's ring buffer, dropping old data
// if the buffer would exceed maxBytes.
func (j *Job) AppendOutput(p []byte) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.maxBytes <= 0 {
		j.maxBytes = 1 << 20 // 1MB default
	}

	if len(j.out)+len(p) > j.maxBytes {
		drop := (len(j.out) + len(p)) - j.maxBytes
		if drop > len(j.out) {
			drop = len(j.out)
		}
		j.out = j.out[drop:]
		j.droppedBytes += int64(drop)
	}
	j.out = append(j.out, p...)
}

// Tail returns the last n lines of output, the number of dropped bytes,
// and whether the output was truncated.
func (j *Job) Tail(maxLines int) (output string, droppedBytes int64, truncatedLines bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if maxLines <= 0 {
		maxLines = 200
	}

	if len(j.out) == 0 {
		return "", j.droppedBytes, false
	}

	lines := bytes.Split(j.out, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}

	truncatedLines = len(lines) > maxLines
	start := 0
	if len(lines) > maxLines {
		start = len(lines) - maxLines
	}

	var result bytes.Buffer
	for i := start; i < len(lines); i++ {
		if i > start {
			result.WriteByte('\n')
		}
		result.Write(lines[i])
	}

	return result.String(), j.droppedBytes, truncatedLines
}

// JobRegistry manages background jobs for an agent.
type JobRegistry struct {
	mu   sync.RWMutex
	jobs map[string]*Job
}

// NewJobRegistry creates a new, empty job registry.
func NewJobRegistry() *JobRegistry {
	return &JobRegistry{
		jobs: make(map[string]*Job),
	}
}

// Start creates a new job with State=JobRunning and returns it along with
// a derived context. The caller holds the context and runs the actual work;
// when done, call Finish to lock in the final state.
func (r *JobRegistry) Start(parent context.Context, kind JobKind, description string) (*Job, context.Context) {
	id := uuid.NewString()
	derived, cancel := context.WithCancel(parent)

	job := &Job{
		ID:          id,
		Kind:        kind,
		Description: description,
		StartedAt:   time.Now(),
		State:       JobRunning,
		cancel:      cancel,
		maxBytes:    1 << 20,
	}

	r.mu.Lock()
	r.jobs[id] = job
	r.mu.Unlock()

	return job, derived
}

// Finish marks a job as completed with the given state and summary.
// It is safe to call multiple times; only the first call takes effect.
func (r *JobRegistry) Finish(id string, state JobState, summary string) {
	r.mu.Lock()
	job, ok := r.jobs[id]
	if !ok {
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()

	job.mu.Lock()
	defer job.mu.Unlock()

	if job.State != JobRunning {
		return
	}

	job.FinishedAt = time.Now()
	job.State = state
	job.Summary = summary

	if job.cancel != nil {
		job.cancel()
	}
}

// Get returns the job with the given ID, or nil if not found.
func (r *JobRegistry) Get(id string) (*Job, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	job, ok := r.jobs[id]
	return job, ok
}

// List returns all jobs in the registry.
func (r *JobRegistry) List() []*Job {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]*Job, 0, len(r.jobs))
	for _, job := range r.jobs {
		out = append(out, job)
	}
	return out
}

// Cancel cancels the job with the given ID. Returns true if the job was
// found and canceled, false otherwise. Calling Cancel on an already-
// cancelled or finished job returns false.
func (r *JobRegistry) Cancel(id string) bool {
	r.mu.RLock()
	job, ok := r.jobs[id]
	r.mu.RUnlock()

	if !ok {
		return false
	}

	job.mu.Lock()
	defer job.mu.Unlock()

	if job.State != JobRunning {
		return false
	}

	if job.cancel != nil {
		job.cancel()
	}
	// Mark as canceled immediately so subsequent Cancel calls return false
	job.State = JobCanceled
	job.FinishedAt = time.Now()
	job.Summary = "canceled by user"
	return true
}

// Close cancels all running jobs and waits up to 2 seconds for them to finish.
func (r *JobRegistry) Close() {
	r.mu.RLock()
	var running []*Job
	for _, job := range r.jobs {
		job.mu.Lock()
		if job.State == JobRunning {
			running = append(running, job)
		}
		job.mu.Unlock()
	}
	r.mu.RUnlock()

	for _, job := range running {
		if job.cancel != nil {
			job.cancel()
		}
	}

	// Wait up to 2 seconds for all jobs to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(running) > 0 {
		var stillRunning []*Job
		for _, job := range running {
			job.mu.Lock()
			if job.State == JobRunning {
				stillRunning = append(stillRunning, job)
			}
			job.mu.Unlock()
		}
		running = stillRunning
		if len(running) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// JobStatus returns status information for a job suitable for tools.Status return.
func (r *JobRegistry) JobStatus(id string) (Status, error) {
	job, ok := r.Get(id)
	if !ok {
		return Status{}, fmt.Errorf("job not found: %s", id)
	}

	// Don't hold lock while calling Tail - get needed values first
	job.mu.Lock()
	jobID := job.ID
	kind := job.Kind
	state := job.State
	startedAt := job.StartedAt
	finishedAt := job.FinishedAt
	summary := job.Summary
	job.mu.Unlock()

	// Now call Tail without holding the lock
	tail, dropped, truncated := job.Tail(200)
	var tailLines int
	if tail != "" {
		tailLines = bytes.Count([]byte(tail), []byte{'\n'}) + 1
	}

	return Status{
		ID:           jobID,
		Kind:         kind.String(),
		State:        state.String(),
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		Summary:      summary,
		Tail:         tail,
		DroppedBytes: dropped,
		TotalLines:   tailLines,
		Truncated:    truncated,
	}, nil
}

// Status is the public status struct returned to tools.
type Status struct {
	ID           string
	Kind         string
	State        string
	StartedAt    time.Time
	FinishedAt   time.Time
	Summary      string
	Tail         string
	DroppedBytes int64
	TotalLines   int
	Truncated    bool
}
