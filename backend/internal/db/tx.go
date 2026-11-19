package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/real-musafir/b-scale/backend/internal/db/sqlc"
)

// Beginner starts a transaction. Both *pgxpool.Pool and pgx.Tx satisfy it:
// the pool opens a real transaction, and a transaction opens a nested one
// backed by a savepoint. That lets a test run the production code path inside
// an outer transaction it then rolls back.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// InTx runs fn inside a single transaction.
//
// The transaction is rolled back unless fn returns nil and the commit
// succeeds, so a partially applied change can never survive: either every
// statement fn issued is visible, or none is.
func InTx(ctx context.Context, db Beginner, fn func(q *sqlc.Queries) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("db: begin transaction: %w", err)
	}

	// Rollback after a successful commit is a no-op, so this is safe on every
	// path, including a panic unwinding through it.
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()

	if err := fn(sqlc.New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("db: commit transaction: %w", err)
	}
	return nil
}
