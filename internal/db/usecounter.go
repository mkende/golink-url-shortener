// Package db provides database access for golink-url-shortener.
package db

import (
	"context"
	"sync"
	"time"
)

// UseCounter batches link use-count increments and flushes them to the
// database periodically via a background goroutine. This avoids write
// contention on the hot redirect path.
type UseCounter struct {
	db      *DB
	mu      sync.Mutex
	counts  map[int64]int64 // link ID → pending increment
	flushCh chan struct{}
	doneCh  chan struct{}
}

// NewUseCounter creates a UseCounter and starts its background flush goroutine.
// Call Shutdown to drain and stop it.
func NewUseCounter(db *DB, flushInterval time.Duration) *UseCounter {
	uc := &UseCounter{
		db:      db,
		counts:  make(map[int64]int64),
		flushCh: make(chan struct{}, 1),
		doneCh:  make(chan struct{}),
	}
	go uc.run(flushInterval)
	return uc
}

// Increment schedules a use-count increment for the given link ID.
// It is safe to call from multiple goroutines.
func (uc *UseCounter) Increment(id int64) {
	uc.mu.Lock()
	uc.counts[id]++
	uc.mu.Unlock()
	// Non-blocking nudge to flush sooner if the channel has capacity.
	select {
	case uc.flushCh <- struct{}{}:
	default:
	}
}

// Shutdown flushes all pending increments and stops the background goroutine.
// It blocks until flushing is complete.
func (uc *UseCounter) Shutdown(ctx context.Context) error {
	close(uc.flushCh)
	select {
	case <-uc.doneCh:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (uc *UseCounter) run(interval time.Duration) {
	defer close(uc.doneCh)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case _, ok := <-uc.flushCh:
			if !ok {
				// Channel closed: final flush then exit.
				uc.flush()
				return
			}
			// Nudged — drain the ticker and flush.
			uc.flush()
		case <-ticker.C:
			uc.flush()
		}
	}
}

func (uc *UseCounter) flush() {
	uc.mu.Lock()
	if len(uc.counts) == 0 {
		uc.mu.Unlock()
		return
	}
	// Swap out the map so we hold the lock as briefly as possible.
	pending := uc.counts
	uc.counts = make(map[int64]int64)
	uc.mu.Unlock()

	ctx := context.Background()
	for id, delta := range pending {
		_, err := uc.db.ExecContext(ctx,
			uc.db.q("UPDATE links SET use_count = use_count + ?, last_used_at = CURRENT_TIMESTAMP WHERE id = ?"),
			delta, id)
		if err != nil {
			// Best-effort; don't crash or retry for stats.
			_ = err
		}
	}
}
