package server_test

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/mkende/golink-redirector/internal/config"
	"github.com/mkende/golink-redirector/internal/db"
	"github.com/mkende/golink-redirector/internal/server"
)

// BenchmarkRedirect measures the redirect path performance including the LRU
// cache hit path (all requests after the first are cache hits).
//
// Example result on a Linux/amd64 dev machine (Intel Xeon @ 2.10GHz):
//
//	BenchmarkRedirect-4   591861   6517 ns/op  (~6.5 µs/op)
func BenchmarkRedirect(b *testing.B) {
	sqlDB, err := db.Open(context.Background(), "sqlite3", ":memory:")
	if err != nil {
		b.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()

	cfg := &config.Config{
		ListenAddr: ":8080",
		Title:      "Bench GoLink",
		CacheSize:  1000,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := server.New(cfg, sqlDB, logger, nil)

	linkRepo := db.NewLinkRepo(sqlDB)
	if _, err := linkRepo.Create(context.Background(), "bench", "https://example.com/bench", "test@example.com", db.LinkTypeSimple, "", false); err != nil {
		b.Fatalf("create link: %v", err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req := httptest.NewRequest("GET", "/bench", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != 302 {
				b.Errorf("expected 302, got %d", w.Code)
			}
		}
	})
}
