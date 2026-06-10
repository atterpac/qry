package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func init() {
	register("mysql", func(ctx context.Context) error {
		dsn := envOr("MYSQL_DSN", "qrytest:qrytest@tcp(localhost:13306)/qrytest?parseTime=true&multiStatements=true")
		db, err := connectMySQL(ctx, dsn)
		if err != nil {
			return err
		}
		defer db.Close()
		return seedMySQL(ctx, db)
	})
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
