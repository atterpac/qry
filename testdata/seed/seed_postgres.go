package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func init() {
	register("postgres", func(ctx context.Context) error {
		dsn := envOr("PG_DSN", "postgres://qrytest:qrytest@localhost:15432/qrytest")
		pool, err := connectPG(ctx, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()
		return seedPostgres(ctx, pool)
	})

	register("postgres-prod", func(ctx context.Context) error {
		dsn := envOr("PG_PROD_DSN", "postgres://qrytest:qrytest@localhost:15433/qrytest_prod")
		pool, err := connectPG(ctx, dsn)
		if err != nil {
			return err
		}
		defer pool.Close()
		return seedPostgresProd(ctx, pool)
	})
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

func seedPostgresProd(ctx context.Context, pool *pgxpool.Pool) error {
	for _, stmt := range pgProdDropStatements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("drop: %w", err)
		}
	}
	for _, stmt := range pgProdCreateStatements {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("create: %w", err)
		}
	}

	// Seed same data (prod schema has no display_name, has phone instead)
	for _, u := range seedUsers {
		if _, err := pool.Exec(ctx,
			"INSERT INTO users (username, email, is_active) VALUES ($1, $2, $3)",
			u.Username, u.Email, u.IsActive,
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

	// No product_tags in prod (table doesn't exist)

	for _, e := range seedEvents {
		if _, err := pool.Exec(ctx,
			"INSERT INTO analytics.events (event_type, user_id, payload) VALUES ($1, $2, $3)",
			e.EventType, e.UserID, e.Payload,
		); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}

	// Seed some reviews (prod-only table)
	reviews := []struct {
		ProductID int
		UserID    int
		Rating    int
		Body      *string
	}{
		{1, 1, 5, ptr("Amazing keyboard, love the switches!")},
		{1, 2, 4, ptr("Great build quality")},
		{8, 5, 5, ptr("Best headphones I've ever owned")},
		{11, 8, 4, nil},
		{12, 12, 5, ptr("My back thanks me every day")},
		{2, 3, 3, ptr("Decent mouse, nothing special")},
		{25, 10, 5, ptr("Perfect for digital art")},
		{14, 7, 4, ptr("Fast and reliable SSD")},
		{9, 6, 4, nil},
		{5, 9, 5, ptr("Beautiful desk lamp")},
	}
	for _, r := range reviews {
		if _, err := pool.Exec(ctx,
			"INSERT INTO reviews (product_id, user_id, rating, body) VALUES ($1, $2, $3, $4)",
			r.ProductID, r.UserID, r.Rating, r.Body,
		); err != nil {
			return fmt.Errorf("insert review: %w", err)
		}
	}

	return nil
}
