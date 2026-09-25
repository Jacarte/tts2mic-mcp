package jobs

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func awaitTerminal(t *testing.T, m *Manager, id string) Snapshot {
	t.Helper()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		got, err := m.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if terminal(got.State) {
			return got
		}
		select {
		case <-deadline:
			t.Fatalf("job did not finish: %+v", got)
		case <-ticker.C:
		}
	}
}

func TestCompletionAndFailure(t *testing.T) {
	m := New(context.Background())
	defer m.Close()
	for _, test := range []struct {
		err  error
		want string
	}{{nil, "completed"}, {errors.New("device missing"), "failed"}} {
		job, err := m.Submit(0, time.Second, func(context.Context) error { return test.err })
		if err != nil {
			t.Fatal(err)
		}
		got := awaitTerminal(t, m, job.ID)
		if got.State != test.want || got.StartedAt == nil || got.FinishedAt == nil {
			t.Fatalf("unexpected job: %+v", got)
		}
	}
}

func TestCancellationAndSerialQueue(t *testing.T) {
	m := New(context.Background())
	defer m.Close()
	started := make(chan struct{})
	first, err := m.Submit(0, time.Second, func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	<-started
	var ran atomic.Bool
	second, err := m.Submit(0, time.Second, func(context.Context) error { ran.Store(true); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Cancel(second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Cancel(first.ID); err != nil {
		t.Fatal(err)
	}
	if got := awaitTerminal(t, m, first.ID); got.State != "cancelled" {
		t.Fatal(got)
	}
	if got := awaitTerminal(t, m, second.ID); got.State != "cancelled" {
		t.Fatal(got)
	}
	if ran.Load() {
		t.Fatal("cancelled queued job ran")
	}
}

func TestTimeoutAndShutdown(t *testing.T) {
	m := New(context.Background())
	job, err := m.Submit(0, 20*time.Millisecond, func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	if got := awaitTerminal(t, m, job.ID); got.State != "failed" || got.Error != context.DeadlineExceeded.Error() {
		t.Fatal(got)
	}
	pending, err := m.Submit(time.Second, 2*time.Second, func(context.Context) error { t.Error("shutdown job ran"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	m.Close()
	if got := awaitTerminal(t, m, pending.ID); got.State != "cancelled" {
		t.Fatal(got)
	}
	if _, err := m.Submit(0, time.Second, func(context.Context) error { return nil }); err == nil {
		t.Fatal("submit after shutdown succeeded")
	}
	m.Close() // Idempotent.
}

func TestBoundedQueueAndInvalidDelay(t *testing.T) {
	m := New(context.Background())
	defer m.Close()
	start := make(chan struct{})
	if _, err := m.Submit(0, time.Minute, func(ctx context.Context) error { close(start); <-ctx.Done(); return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	<-start
	for i := 0; i < queueSize; i++ {
		if _, err := m.Submit(0, time.Minute, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Submit(0, time.Minute, func(context.Context) error { return nil }); err == nil {
		t.Fatal("queue not bounded")
	}
	if _, err := m.Submit(-time.Second, time.Minute, func(context.Context) error { return nil }); err == nil {
		t.Fatal("negative delay accepted")
	}
}
