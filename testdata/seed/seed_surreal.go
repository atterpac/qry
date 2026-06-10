package main

// SurrealDB is the worked example of adding a brand-new engine to the test
// harness. The full loop is four edits:
//
//  1. Add a service to testdata/docker-compose.yml (the `surrealdb` service).
//  2. Add this file: connect (with retry) + define schema + insert seed data.
//  3. Add a profile to testdata/config.yaml + testdata/qry-test.yaml.
//  4. Nothing else — `register(...)` below wires it into `task testdb-seed`.
//
// Run just this one:  go run ./testdata/seed/ surrealdb

import (
	"context"
	"fmt"
	"time"

	surrealdb "github.com/surrealdb/surrealdb.go"
)

func init() {
	register("surrealdb", func(ctx context.Context) error {
		url := envOr("SURREAL_URL", "ws://localhost:18000/rpc")
		user := envOr("SURREAL_USER", "root")
		pass := envOr("SURREAL_PASS", "root")
		ns := envOr("SURREAL_NS", "qrytest")
		dbName := envOr("SURREAL_DB", "qrytest")

		db, err := connectSurreal(ctx, url, user, pass, ns, dbName)
		if err != nil {
			return err
		}
		defer db.Close(ctx)
		return seedSurreal(ctx, db)
	})
}

func connectSurreal(ctx context.Context, url, user, pass, ns, dbName string) (*surrealdb.DB, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		db, err := surrealdb.New(url)
		if err == nil {
			_, err = db.SignIn(ctx, &surrealdb.Auth{Username: user, Password: pass})
			if err == nil {
				if err = db.Use(ctx, ns, dbName); err == nil {
					return db, nil
				}
			}
			db.Close(ctx)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out connecting to surrealdb: %v", err)
		}
		time.Sleep(1 * time.Second)
	}
}

func seedSurreal(ctx context.Context, db *surrealdb.DB) error {
	// SurrealDB is schemaless-friendly, but we declare SCHEMAFULL tables so the
	// app's schema browser has real field definitions to show, mirroring the
	// relational seed shape (users / products / orders).
	schema := `
		REMOVE TABLE IF EXISTS users;
		REMOVE TABLE IF EXISTS products;
		REMOVE TABLE IF EXISTS orders;

		DEFINE TABLE users SCHEMAFULL;
		DEFINE FIELD username     ON users TYPE string;
		DEFINE FIELD email        ON users TYPE string;
		DEFINE FIELD display_name ON users TYPE option<string>;
		DEFINE FIELD is_active    ON users TYPE bool DEFAULT true;
		DEFINE INDEX users_username ON users FIELDS username UNIQUE;

		DEFINE TABLE products SCHEMAFULL;
		DEFINE FIELD name        ON products TYPE string;
		DEFINE FIELD category    ON products TYPE string;
		DEFINE FIELD price       ON products TYPE number;
		DEFINE FIELD sku         ON products TYPE string;
		DEFINE FIELD description  ON products TYPE option<string>;
		DEFINE FIELD in_stock    ON products TYPE bool DEFAULT true;

		DEFINE TABLE orders SCHEMAFULL;
		DEFINE FIELD user   ON orders TYPE record<users>;
		DEFINE FIELD status ON orders TYPE string;
		DEFINE FIELD total  ON orders TYPE number;
		DEFINE FIELD notes  ON orders TYPE option<string>;
	`
	if _, err := surrealdb.Query[any](ctx, db, schema, nil); err != nil {
		return fmt.Errorf("surreal schema: %w", err)
	}

	// Insert users, keying the record id by array index so orders can link them.
	userIDs := make([]string, len(seedUsers))
	for i, u := range seedUsers {
		id := fmt.Sprintf("users:u%d", i+1)
		userIDs[i] = id
		vars := map[string]any{
			"username":     u.Username,
			"email":        u.Email,
			"display_name": deref(u.DisplayName),
			"is_active":    u.IsActive,
		}
		q := fmt.Sprintf("CREATE %s CONTENT { username: $username, email: $email, display_name: $display_name, is_active: $is_active };", id)
		if _, err := surrealdb.Query[any](ctx, db, q, vars); err != nil {
			return fmt.Errorf("insert user %s: %w", u.Username, err)
		}
	}

	for i, p := range seedProducts {
		id := fmt.Sprintf("products:p%d", i+1)
		vars := map[string]any{
			"name":        p.Name,
			"category":    p.Category,
			"price":       p.Price,
			"sku":         p.SKU,
			"description": deref(p.Description),
			"in_stock":    p.InStock,
		}
		q := fmt.Sprintf("CREATE %s CONTENT { name: $name, category: $category, price: $price, sku: $sku, description: $description, in_stock: $in_stock };", id)
		if _, err := surrealdb.Query[any](ctx, db, q, vars); err != nil {
			return fmt.Errorf("insert product %s: %w", p.Name, err)
		}
	}

	for i, o := range seedOrders {
		// seedOrders.UserID is 1-based into seedUsers.
		if o.UserID < 1 || o.UserID > len(userIDs) {
			continue
		}
		vars := map[string]any{
			"user":   userIDs[o.UserID-1],
			"status": o.Status,
			"total":  o.Total,
			"notes":  deref(o.Notes),
		}
		q := fmt.Sprintf("CREATE orders:o%d CONTENT { user: type::thing($user), status: $status, total: $total, notes: $notes };", i+1)
		if _, err := surrealdb.Query[any](ctx, db, q, vars); err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
	}

	return nil
}

// deref returns the pointed-to string or nil, so nil columns become SurrealDB NONE.
func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}
