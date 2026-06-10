package main

import (
	"context"
	"os"
)

// Seeder knows how to (re)create and populate one test database.
//
// Adding a new database engine is intentionally small: drop a new file in this
// package with an init() that calls register(...). See seed_surreal.go for a
// worked example. The Name() must match the docker-compose service / config
// profile so the whole loop (compose → seed → connect in app) lines up.
type Seeder interface {
	// Name is the short identifier used for CLI filtering and logging.
	// Keep it in sync with the docker-compose service and the config profile.
	Name() string
	// Seed connects (with retry), drops existing objects, recreates the schema,
	// and inserts the shared seed data. It must be idempotent.
	Seed(ctx context.Context) error
}

// seederFunc adapts a plain function into a Seeder.
type seederFunc struct {
	name string
	fn   func(ctx context.Context) error
}

func (s seederFunc) Name() string                    { return s.name }
func (s seederFunc) Seed(ctx context.Context) error  { return s.fn(ctx) }

// registry holds every seeder registered via init(). Order is not significant.
var registry []Seeder

// register adds a seeder to the global registry. Call it from a file-level
// init() so the seeder is wired up just by existing in the package.
func register(name string, fn func(ctx context.Context) error) {
	registry = append(registry, seederFunc{name: name, fn: fn})
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
