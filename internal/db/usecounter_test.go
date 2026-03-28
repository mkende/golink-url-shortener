package db

import (
	"context"
	"testing"
	"time"
)

func TestUseCounter_Increment(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			link, err := repo.Create(ctx, "counter-test", "https://example.com", "o@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			uc := NewUseCounter(b.db, 50*time.Millisecond)

			// Increment multiple times.
			for i := 0; i < 5; i++ {
				uc.Increment(link.ID)
			}

			// Shutdown flushes all pending counts.
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := uc.Shutdown(shutdownCtx); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}

			got, err := repo.GetByName(ctx, "counter-test")
			if err != nil {
				t.Fatalf("GetByName: %v", err)
			}
			if got.UseCount != 5 {
				t.Errorf("UseCount = %d, want 5", got.UseCount)
			}
			if !got.LastUsedAt.Valid {
				t.Error("expected LastUsedAt to be set after flush")
			}
		})
	}
}

func TestUseCounter_Shutdown_Flushes(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			link, err := repo.Create(ctx, "flush-test", "https://example.com", "o@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			// Use a long interval so the ticker won't fire before Shutdown.
			uc := NewUseCounter(b.db, 10*time.Minute)
			uc.Increment(link.ID)
			uc.Increment(link.ID)
			uc.Increment(link.ID)

			// Verify not yet flushed (no tick has fired).
			before, err := repo.GetByName(ctx, "flush-test")
			if err != nil {
				t.Fatalf("GetByName before flush: %v", err)
			}
			if before.UseCount != 0 {
				t.Errorf("UseCount before flush = %d, want 0", before.UseCount)
			}

			// Shutdown must flush before returning.
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := uc.Shutdown(shutdownCtx); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}

			after, err := repo.GetByName(ctx, "flush-test")
			if err != nil {
				t.Fatalf("GetByName after flush: %v", err)
			}
			if after.UseCount != 3 {
				t.Errorf("UseCount after flush = %d, want 3", after.UseCount)
			}
		})
	}
}

func TestUseCounter_Ticker_Flush(t *testing.T) {
	for _, b := range allBackends(t) {
		t.Run(b.name, func(t *testing.T) {
			repo := NewLinkRepo(b.db)
			ctx := context.Background()

			link, err := repo.Create(ctx, "ticker-test", "https://example.com", "o@example.com", LinkTypeSimple, "", false)
			if err != nil {
				t.Fatalf("Create: %v", err)
			}

			// Use a short interval so the ticker fires quickly.
			uc := NewUseCounter(b.db, 20*time.Millisecond)
			uc.Increment(link.ID)
			uc.Increment(link.ID)

			// Wait for at least two ticker intervals.
			time.Sleep(100 * time.Millisecond)

			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			if err := uc.Shutdown(shutdownCtx); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}

			got, err := repo.GetByName(ctx, "ticker-test")
			if err != nil {
				t.Fatalf("GetByName: %v", err)
			}
			if got.UseCount != 2 {
				t.Errorf("UseCount = %d, want 2", got.UseCount)
			}
		})
	}
}
