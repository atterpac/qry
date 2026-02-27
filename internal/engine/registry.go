package engine

import (
	"fmt"

	"github.com/atterpac/qry/internal/config"
)

// NewProvider creates a Provider for the given engine type.
func NewProvider(engineType config.EngineType) (Provider, error) {
	switch engineType {
	case config.EngineSQLite:
		return &SQLiteProvider{}, nil
	case config.EnginePostgres:
		return &PostgresProvider{}, nil
	case config.EngineMySQL:
		return &MySQLProvider{}, nil
	case config.EngineSurrealDB:
		return &SurrealDBProvider{}, nil
	default:
		return nil, fmt.Errorf("unsupported engine: %s", engineType)
	}
}
