package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "modernc.org/sqlite"
)

func main() {
	ctx := context.Background()

	pgDSN := envOr("PG_DSN", "postgres://qrytest:qrytest@localhost:15432/qrytest")
	myDSN := envOr("MYSQL_DSN", "qrytest:qrytest@tcp(localhost:13306)/qrytest?parseTime=true&multiStatements=true")
	sqlitePath := envOr("SQLITE_PATH", "testdata/test.db")

	// Seed Postgres
	log.Println("Connecting to Postgres...")
	pool, err := connectPG(ctx, pgDSN)
	if err != nil {
		log.Fatalf("postgres connect: %v", err)
	}
	defer pool.Close()
	log.Println("Seeding Postgres...")
	if err := seedPostgres(ctx, pool); err != nil {
		log.Fatalf("postgres seed: %v", err)
	}
	log.Println("Postgres seeded.")

	// Seed MySQL
	log.Println("Connecting to MySQL...")
	myDB, err := connectMySQL(ctx, myDSN)
	if err != nil {
		log.Fatalf("mysql connect: %v", err)
	}
	defer myDB.Close()
	log.Println("Seeding MySQL...")
	if err := seedMySQL(ctx, myDB); err != nil {
		log.Fatalf("mysql seed: %v", err)
	}
	log.Println("MySQL seeded.")

	// Seed SQLite
	log.Println("Seeding SQLite...")
	if err := seedSQLite(ctx, sqlitePath); err != nil {
		log.Fatalf("sqlite seed: %v", err)
	}
	log.Println("SQLite seeded.")

	fmt.Println("All databases seeded successfully.")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func connectPG(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		pool, err := pgxpool.New(ctx, dsn)
		if err == nil {
			if err := pool.Ping(ctx); err == nil {
				return pool, nil
			}
			pool.Close()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out connecting to postgres: %v", err)
		}
		time.Sleep(1 * time.Second)
	}
}

func connectMySQL(ctx context.Context, dsn string) (*sql.DB, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		db, err := sql.Open("mysql", dsn)
		if err == nil {
			if err := db.PingContext(ctx); err == nil {
				return db, nil
			}
			db.Close()
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out connecting to mysql: %v", err)
		}
		time.Sleep(1 * time.Second)
	}
}

// ── Postgres ──────────────────────────────────────────────────────────────────

func seedPostgres(ctx context.Context, pool *pgxpool.Pool) error {
	for _, stmt := range pgDropStatements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("drop: %w", err)
		}
	}
	for _, stmt := range pgCreateStatements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("create: %w", err)
		}
	}

	for _, u := range seedUsers {
		if _, err := pool.Exec(ctx,
			"INSERT INTO users (username, email, display_name, is_active) VALUES ($1, $2, $3, $4)",
			u.Username, u.Email, u.DisplayName, u.IsActive,
		); err != nil {
			return fmt.Errorf("insert user %s: %w", u.Username, err)
		}
	}

	for _, p := range seedProducts {
		if _, err := pool.Exec(ctx,
			"INSERT INTO products (name, category, price, sku, description, in_stock) VALUES ($1, $2, $3, $4, $5, $6)",
			p.Name, p.Category, p.Price, p.SKU, p.Description, p.InStock,
		); err != nil {
			return fmt.Errorf("insert product %s: %w", p.Name, err)
		}
	}

	for _, t := range seedTags {
		if _, err := pool.Exec(ctx,
			"INSERT INTO tags (name) VALUES ($1)", t.Name,
		); err != nil {
			return fmt.Errorf("insert tag %s: %w", t.Name, err)
		}
	}

	for _, o := range seedOrders {
		if _, err := pool.Exec(ctx,
			"INSERT INTO orders (user_id, status, total, notes) VALUES ($1, $2, $3, $4)",
			o.UserID, o.Status, o.Total, o.Notes,
		); err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
	}

	for _, oi := range seedOrderItems {
		if _, err := pool.Exec(ctx,
			"INSERT INTO order_items (order_id, product_id, quantity, unit_price) VALUES ($1, $2, $3, $4)",
			oi.OrderID, oi.ProductID, oi.Quantity, oi.UnitPrice,
		); err != nil {
			return fmt.Errorf("insert order_item: %w", err)
		}
	}

	for _, pt := range seedProductTags {
		if _, err := pool.Exec(ctx,
			"INSERT INTO product_tags (product_id, tag_id) VALUES ($1, $2)",
			pt.ProductID, pt.TagID,
		); err != nil {
			return fmt.Errorf("insert product_tag: %w", err)
		}
	}

	for _, e := range seedEvents {
		if _, err := pool.Exec(ctx,
			"INSERT INTO analytics.events (event_type, user_id, payload) VALUES ($1, $2, $3)",
			e.EventType, e.UserID, e.Payload,
		); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}

	return nil
}

// ── MySQL ─────────────────────────────────────────────────────────────────────

func seedMySQL(ctx context.Context, db *sql.DB) error {
	// Disable FK checks for clean drop
	if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		return err
	}
	for _, stmt := range mysqlDropStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1"); err != nil {
		return err
	}

	for _, stmt := range mysqlCreateStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create: %w", err)
		}
	}

	for _, u := range seedUsers {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO users (username, email, display_name, is_active) VALUES (?, ?, ?, ?)",
			u.Username, u.Email, u.DisplayName, u.IsActive,
		); err != nil {
			return fmt.Errorf("insert user %s: %w", u.Username, err)
		}
	}

	for _, p := range seedProducts {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO products (name, category, price, sku, description, in_stock) VALUES (?, ?, ?, ?, ?, ?)",
			p.Name, p.Category, p.Price, p.SKU, p.Description, p.InStock,
		); err != nil {
			return fmt.Errorf("insert product %s: %w", p.Name, err)
		}
	}

	for _, t := range seedTags {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO tags (name) VALUES (?)", t.Name,
		); err != nil {
			return fmt.Errorf("insert tag %s: %w", t.Name, err)
		}
	}

	for _, o := range seedOrders {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO orders (user_id, status, total, notes) VALUES (?, ?, ?, ?)",
			o.UserID, o.Status, o.Total, o.Notes,
		); err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
	}

	for _, oi := range seedOrderItems {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO order_items (order_id, product_id, quantity, unit_price) VALUES (?, ?, ?, ?)",
			oi.OrderID, oi.ProductID, oi.Quantity, oi.UnitPrice,
		); err != nil {
			return fmt.Errorf("insert order_item: %w", err)
		}
	}

	for _, pt := range seedProductTags {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO product_tags (product_id, tag_id) VALUES (?, ?)",
			pt.ProductID, pt.TagID,
		); err != nil {
			return fmt.Errorf("insert product_tag: %w", err)
		}
	}

	return nil
}

// ── SQLite ────────────────────────────────────────────────────────────────────

func seedSQLite(ctx context.Context, path string) error {
	os.Remove(path) // start fresh

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open sqlite: %w", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return err
	}

	for _, stmt := range sqliteDropStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("drop: %w", err)
		}
	}

	for _, stmt := range sqliteCreateStatements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create: %w", err)
		}
	}

	for _, u := range seedUsers {
		active := 0
		if u.IsActive {
			active = 1
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO users (username, email, display_name, is_active) VALUES (?, ?, ?, ?)",
			u.Username, u.Email, u.DisplayName, active,
		); err != nil {
			return fmt.Errorf("insert user %s: %w", u.Username, err)
		}
	}

	for _, p := range seedProducts {
		stock := 0
		if p.InStock {
			stock = 1
		}
		if _, err := db.ExecContext(ctx,
			"INSERT INTO products (name, category, price, sku, description, in_stock) VALUES (?, ?, ?, ?, ?, ?)",
			p.Name, p.Category, p.Price, p.SKU, p.Description, stock,
		); err != nil {
			return fmt.Errorf("insert product %s: %w", p.Name, err)
		}
	}

	for _, t := range seedTags {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO tags (name) VALUES (?)", t.Name,
		); err != nil {
			return fmt.Errorf("insert tag %s: %w", t.Name, err)
		}
	}

	for _, o := range seedOrders {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO orders (user_id, status, total, notes) VALUES (?, ?, ?, ?)",
			o.UserID, o.Status, o.Total, o.Notes,
		); err != nil {
			return fmt.Errorf("insert order: %w", err)
		}
	}

	for _, oi := range seedOrderItems {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO order_items (order_id, product_id, quantity, unit_price) VALUES (?, ?, ?, ?)",
			oi.OrderID, oi.ProductID, oi.Quantity, oi.UnitPrice,
		); err != nil {
			return fmt.Errorf("insert order_item: %w", err)
		}
	}

	for _, pt := range seedProductTags {
		if _, err := db.ExecContext(ctx,
			"INSERT INTO product_tags (product_id, tag_id) VALUES (?, ?)",
			pt.ProductID, pt.TagID,
		); err != nil {
			return fmt.Errorf("insert product_tag: %w", err)
		}
	}

	return nil
}
