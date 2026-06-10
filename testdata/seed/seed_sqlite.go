package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func init() {
	register("sqlite", func(ctx context.Context) error {
		path := envOr("SQLITE_PATH", "testdata/test.db")
		return seedSQLite(ctx, path)
	})
}

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
