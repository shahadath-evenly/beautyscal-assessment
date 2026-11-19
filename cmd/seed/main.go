// Command seed loads idempotent demo data: two workspaces and a small
// catalogue each, printing the workspace IDs for frontend demo configuration.
//
//	go run ./cmd/seed             insert anything missing, leave existing rows
//	go run ./cmd/seed -refresh    also reset prices, currencies and stock
//
// Running it repeatedly never duplicates a workspace or a product. It touches
// no order requests or orders, so it cannot interfere with the accept flow.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/real-musafir/b-scale/backend/internal/config"
	"github.com/real-musafir/b-scale/backend/internal/db"
	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
	"github.com/real-musafir/b-scale/backend/internal/platform/logging"
	"github.com/real-musafir/b-scale/backend/internal/platform/startup"
	"github.com/real-musafir/b-scale/backend/internal/seed"
)

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	refresh := flag.Bool("refresh", false,
		"reset price, currency and stock of products that already exist")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg.LogLevel, cfg.LogFormat)
	human := cfg.LogFormat == "text"
	report := startup.New(logger, human)
	report.EnvLoaded(cfg.EnvFile, cfg.EnvInjected)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, db.PoolConfig{
		DatabaseURL:    cfg.DatabaseURL,
		MaxConns:       cfg.DBMaxConns,
		MinConns:       cfg.DBMinConns,
		ConnectTimeout: cfg.DBConnectTimeout,
	})
	if err != nil {
		return err
	}
	defer pool.Close()
	report.DatabaseConnected()

	// One transaction for the whole seed: either every workspace and product
	// lands, or none does.
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("seed: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	results, err := seed.Run(ctx, sqlc.New(tx), seed.Options{Refresh: *refresh})
	if err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("seed: commit: %w", err)
	}

	if human {
		printSummary(results)
	} else {
		for _, r := range results {
			logger.Info("workspace seeded",
				"id", r.ID, "name", r.Name, "slug", r.Slug,
				"created", r.Created, "updated", r.Updated,
				"existing", r.Existing, "products", r.Total)
		}
	}
	return nil
}

// printSummary prints the workspace IDs the frontend needs for its demo
// workspace selector, with the per-workspace product counts.
func printSummary(results []seed.WorkspaceResult) {
	fmt.Println()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKSPACE\tSLUG\tID\tPRODUCTS\tTHIS RUN")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			r.Name, r.Slug, r.ID, r.Total, changeSummary(r))
	}
	_ = w.Flush()

	fmt.Println()
	fmt.Println("Use an ID above as the X-Workspace-ID header, or pick the workspace in the UI.")
}

func changeSummary(r seed.WorkspaceResult) string {
	switch {
	case r.Created == 0 && r.Updated == 0:
		return "no changes"
	case r.Updated == 0:
		return fmt.Sprintf("%d created", r.Created)
	case r.Created == 0:
		return fmt.Sprintf("%d refreshed", r.Updated)
	default:
		return fmt.Sprintf("%d created, %d refreshed", r.Created, r.Updated)
	}
}
