package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"github.com/blackrelay/registry/internal/config"
	"github.com/blackrelay/registry/internal/db"
)

func main() {
	cfg := config.Load()
	databaseURL := flag.String("database-url", cfg.DatabaseURL, "PostgreSQL connection string")
	timeout := flag.Duration("timeout", 30*time.Second, "migration timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	pool, err := db.Connect(ctx, *databaseURL)
	if err != nil {
		slog.Error("connect PostgreSQL", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.ApplyMigrations(ctx, pool); err != nil {
		slog.Error("apply migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")
}
