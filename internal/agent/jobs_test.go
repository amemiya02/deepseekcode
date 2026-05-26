package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestJobRegistryStart(t *testing.T) {
	r := NewJobRegistry()
	job, ctx := r.Start(context.Background(), JobSubagent, "test job")

	if job.ID == "" {
		t.Error("job ID should not be empty")
	}
	if job.State != JobRunning {
		t.Errorf("expected JobRunning, got %v", job.State)
	}
	if job.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
	if ctx == nil {
		t.Error("context should not be nil")
	}

	// Verify job is in registry
	got, ok := r.Get(job.ID)
	if !ok {
		t.Error("job not found in registry")
	}
	if got != job {
		t.Error("retrieved job does not match")
	}
}

func TestJobRegistryFinish(t *testing.T) {
	r := NewJobRegistry()
	job, _ := r.Start(context.Background(), JobBackgroundBash, "test")

	r.Finish(job.ID, JobSucceeded, "all done")

	if job.State != JobSucceeded {
		t.Errorf("expected JobSucceeded, got %v", job.State)
	}
	if job.FinishedAt.IsZero() {
		t.Error("FinishedAt should be set")
	}
	if job.Summary != "all done" {
		t.Errorf("expected summary 'all done', got %q", job.Summary)
	}

	// Verify cancel function was set
	if job.cancel == nil {
		t.Error("cancel function should be set")
	}
}

func TestJobRingBuffer(t *testing.T) {
	job := &Job{
		maxBytes: 100,
	}

	// Write within limit
	job.AppendOutput([]byte("hello"))
	if len(job.out) != 5 {
		t.Errorf("expected 5 bytes, got %d", len(job.out))
	}

	// Write that exceeds limit
	job.AppendOutput([]byte(strings.Repeat("x", 100)))
	if len(job.out) > 100 {
		t.Errorf("expected at most 100 bytes, got %d", len(job.out))
	}
	if job.droppedBytes == 0 {
		t.Error("expected some dropped bytes")
	}

	// Verify tail returns data
	tail, dropped, _ := job.Tail(10)
	if tail == "" {
		t.Error("tail should not be empty")
	}
	if dropped != job.droppedBytes {
		t.Errorf("dropped mismatch: got %d, want %d", dropped, job.droppedBytes)
	}
}

func TestJobTailLines(t *testing.T) {
	job := &Job{
		maxBytes: 1 << 20,
	}

	// Write multiple lines
	job.AppendOutput([]byte("line1\nline2\nline3\nline4\nline5"))

	// Request fewer lines than total
	tail, _, truncated := job.Tail(3)
	if truncated != true {
		t.Error("expected truncated=true")
	}
	if !strings.Contains(tail, "line3") {
		t.Errorf("expected tail to contain 'line3', got %q", tail)
	}
	if strings.Contains(tail, "line1") {
		t.Errorf("expected tail to NOT contain 'line1', got %q", tail)
	}
}

func TestJobRegistryCancel(t *testing.T) {
	r := NewJobRegistry()
	job, ctx := r.Start(context.Background(), JobSubagent, "test")

	// Cancel the job
	ok := r.Cancel(job.ID)
	if !ok {
		t.Error("expected Cancel to return true")
	}

	// Context should be cancelled
	select {
	case <-ctx.Done():
		// Good
	case <-time.After(100 * time.Millisecond):
		t.Error("context should be cancelled")
	}

	// Cancel again should return false
	ok = r.Cancel(job.ID)
	if ok {
		t.Error("expected Cancel to return false for already-cancelled job")
	}

	// Cancel unknown job
	ok = r.Cancel("nonexistent")
	if ok {
		t.Error("expected Cancel to return false for unknown job")
	}
}

func TestJobRegistryClose(t *testing.T) {
	r := NewJobRegistry()
	job1, _ := r.Start(context.Background(), JobSubagent, "job1")
	job2, _ := r.Start(context.Background(), JobBackgroundBash, "job2")

	// Finish one job before Close
	r.Finish(job1.ID, JobSucceeded, "done")

	r.Close()

	// Both jobs should not be running
	if job1.State != JobSucceeded {
		t.Errorf("job1 should still be succeeded, got %v", job1.State)
	}
	if job2.State != JobRunning {
		t.Errorf("job2 should be canceled by Close, got %v", job2.State)
	}
}

func TestJobRegistryList(t *testing.T) {
	r := NewJobRegistry()

	// Empty registry
	list := r.List()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d", len(list))
	}

	// Add jobs
	r.Start(context.Background(), JobSubagent, "job1")
	r.Start(context.Background(), JobBackgroundBash, "job2")

	list = r.List()
	if len(list) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(list))
	}
}

func TestJobKindString(t *testing.T) {
	tests := []struct {
		kind JobKind
		want string
	}{
		{JobSubagent, "subagent"},
		{JobBackgroundBash, "background_bash"},
		{JobKind(99), "unknown"},
	}

	for _, tc := range tests {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("JobKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestJobStateString(t *testing.T) {
	tests := []struct {
		state JobState
		want  string
	}{
		{JobRunning, "running"},
		{JobSucceeded, "succeeded"},
		{JobFailed, "failed"},
		{JobCanceled, "canceled"},
		{JobState(99), "unknown"},
	}

	for _, tc := range tests {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("JobState(%d).String() = %q, want %q", tc.state, got, tc.want)
		}
	}
}

func TestJobRegistryConcurrentAccess(t *testing.T) {
	r := NewJobRegistry()
	var wg sync.WaitGroup

	// Concurrent starts
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, _ := r.Start(context.Background(), JobSubagent, "test")
			r.Finish(job.ID, JobSucceeded, "done")
		}()
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.List()
		}()
	}

	wg.Wait()
}

func TestJobRegistryJobStatus(t *testing.T) {
	r := NewJobRegistry()
	job, _ := r.Start(context.Background(), JobSubagent, "test job")
	job.AppendOutput([]byte("output line 1\noutput line 2"))

	r.Finish(job.ID, JobSucceeded, "completed")

	status, err := r.JobStatus(job.ID)
	if err != nil {
		t.Fatalf("JobStatus error: %v", err)
	}

	if status.ID != job.ID {
		t.Errorf("ID mismatch: got %q, want %q", status.ID, job.ID)
	}
	if status.Kind != "subagent" {
		t.Errorf("Kind mismatch: got %q, want %q", status.Kind, "subagent")
	}
	if status.State != "succeeded" {
		t.Errorf("State mismatch: got %q, want %q", status.State, "succeeded")
	}
	if status.Summary != "completed" {
		t.Errorf("Summary mismatch: got %q, want %q", status.Summary, "completed")
	}
	if !strings.Contains(status.Tail, "output line") {
		t.Errorf("Tail should contain output: %q", status.Tail)
	}

	// Unknown job
	_, err = r.JobStatus("nonexistent")
	if err == nil {
		t.Error("expected error for unknown job")
	}
}
