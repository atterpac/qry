// Command seed (re)creates and populates the qry test databases.
//
// Usage:
//
//	go run ./testdata/seed/                 # seed every registered database
//	go run ./testdata/seed/ postgres mysql  # seed only the named ones
//
// Each database is a Seeder registered in its own file (seed_postgres.go,
// seed_mysql.go, seed_sqlite.go, seed_surreal.go). DSNs are overridable via env
// vars (PG_DSN, MYSQL_DSN, SQLITE_PATH, SURREAL_URL, ...).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
)

func main() {
	ctx := context.Background()

	// Optional positional args filter which seeders run (by Name()).
	want := map[string]bool{}
	for _, a := range os.Args[1:] {
		want[a] = true
	}

	var ran int
	for _, s := range registry {
		if len(want) > 0 && !want[s.Name()] {
			continue
		}
		log.Printf("[%s] seeding...", s.Name())
		if err := s.Seed(ctx); err != nil {
			log.Fatalf("[%s] FAILED: %v", s.Name(), err)
		}
		log.Printf("[%s] done.", s.Name())
		ran++
	}

	if ran == 0 {
		log.Fatalf("no seeders matched %v (registered: %s)", os.Args[1:], registeredNames())
	}
	fmt.Printf("Seeded %d database(s) successfully.\n", ran)
}

func registeredNames() string {
	names := make([]string, len(registry))
	for i, s := range registry {
		names[i] = s.Name()
	}
	return fmt.Sprint(names)
}
