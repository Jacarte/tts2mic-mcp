// Package jobs supervises bounded, serial audio jobs. Completion means the
// backend returned, not proof that a browser captured or audibly rendered audio.
package jobs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

const queueSize = 16
const historySize = 128

type Snapshot struct {
	ID          string     `json:"id"`
	State       string     `json:"state"`
	SubmittedAt time.Time  `json:"submitted_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
	Error       string     `json:"error,omitempty"`
}

type entry struct {
	snapshot Snapshot
	ctx      context.Context
	cancel   context.CancelFunc
	delay    time.Duration
	work     func(context.Context) error
}

type Manager struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	queue   chan *entry
	entries map[string]*entry
	history []string
	done    chan struct{}
}

func New(parent context.Context) *Manager {
	ctx, cancel := context.WithCancel(parent)
	m := &Manager{ctx: ctx, cancel: cancel, queue: make(chan *entry, queueSize), entries: make(map[string]*entry), done: make(chan struct{})}
	go m.run()
	return m
}

// Submit bounds queueing, delay and execution by timeout from submission time.
// Delay is applied after the job reaches the head of the serial queue.
func (m *Manager) Submit(delay, timeout time.Duration, work func(context.Context) error) (Snapshot, error) {
	if delay < 0 || timeout <= 0 || delay >= timeout || work == nil {
		return Snapshot{}, errors.New("invalid job delay, timeout or work")
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Snapshot{}, err
	}
	ctx, cancel := context.WithTimeout(m.ctx, timeout)
	e := &entry{snapshot: Snapshot{ID: hex.EncodeToString(id[:]), State: "queued", SubmittedAt: time.Now().UTC()}, ctx: ctx, cancel: cancel, delay: delay, work: work}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		cancel()
		return Snapshot{}, errors.New("audio job manager is closed")
	}
	m.entries[e.snapshot.ID] = e
	select {
	case m.queue <- e:
		return e.snapshot, nil
	default:
		delete(m.entries, e.snapshot.ID)
		cancel()
		return Snapshot{}, errors.New("audio job queue is full")
	}
}

func (m *Manager) Get(id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return Snapshot{}, fmt.Errorf("unknown or expired job %q", id)
	}
	return e.snapshot, nil
}

func terminal(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled"
}

func (m *Manager) Cancel(id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return Snapshot{}, fmt.Errorf("unknown or expired job %q", id)
	}
	if !terminal(e.snapshot.State) {
		e.cancel()
		e.snapshot.State = "cancelling"
	}
	return e.snapshot, nil
}

// Close cancels current and queued jobs and waits for backend cleanup.
// Work functions must honor their context; subprocess backends use CommandContext.
func (m *Manager) Close() { m.cancel(); <-m.done }

func (m *Manager) run() {
	defer close(m.done)
	for {
		select {
		case e := <-m.queue:
			m.execute(e)
		case <-m.ctx.Done():
			// Submit cannot add entries after cancellation. Drain without playing.
			m.mu.Lock()
			for {
				select {
				case e := <-m.queue:
					e.cancel()
					m.finishLocked(e, context.Canceled)
				default:
					m.mu.Unlock()
					return
				}
			}
		}
	}
}

func (m *Manager) execute(e *entry) {
	defer e.cancel()
	timer := time.NewTimer(e.delay)
	defer timer.Stop()
	var err error
	select {
	case <-e.ctx.Done():
		err = e.ctx.Err()
	case <-timer.C:
		m.mu.Lock()
		if err = e.ctx.Err(); err == nil {
			now := time.Now().UTC()
			e.snapshot.State, e.snapshot.StartedAt = "running", &now
		}
		m.mu.Unlock()
		if err == nil {
			err = e.work(e.ctx)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e.ctx.Err() != nil {
		err = e.ctx.Err()
	}
	m.finishLocked(e, err)
}

func (m *Manager) finishLocked(e *entry, err error) {
	now := time.Now().UTC()
	e.snapshot.FinishedAt = &now
	e.snapshot.State = "completed"
	if errors.Is(err, context.Canceled) {
		e.snapshot.State = "cancelled"
	} else if err != nil {
		e.snapshot.State = "failed"
	}
	if err != nil {
		e.snapshot.Error = err.Error()
	}
	e.work = nil // Do not retain text/audio closures with history.
	m.history = append(m.history, e.snapshot.ID)
	if len(m.history) > historySize {
		delete(m.entries, m.history[0])
		m.history = m.history[1:]
	}
}
