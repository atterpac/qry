package engine

import (
	"context"
	"database/sql"
)

// TransactionalProvider is an optional interface for providers that support
// explicit transaction management.
type TransactionalProvider interface {
	BeginTx(ctx context.Context) error
	CommitTx(ctx context.Context) error
	RollbackTx(ctx context.Context) error
	InTransaction() bool
}

// sqlQueryer is the common interface between *sql.DB and *sql.Tx.
type sqlQueryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}
