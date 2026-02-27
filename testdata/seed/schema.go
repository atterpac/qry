package main

// ── Postgres DDL ──────────────────────────────────────────────────────────────

var pgDropStatements = []string{
	"DROP TABLE IF EXISTS analytics.events",
	"DROP SCHEMA IF EXISTS analytics CASCADE",
	"DROP TABLE IF EXISTS order_items",
	"DROP TABLE IF EXISTS product_tags",
	"DROP TABLE IF EXISTS orders",
	"DROP TABLE IF EXISTS products",
	"DROP TABLE IF EXISTS tags",
	"DROP TABLE IF EXISTS users",
}

var pgCreateStatements = []string{
	`CREATE TABLE users (
		id SERIAL PRIMARY KEY,
		username VARCHAR(50) NOT NULL UNIQUE,
		email VARCHAR(100) NOT NULL UNIQUE,
		display_name VARCHAR(100),
		is_active BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMP NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP
	)`,
	`CREATE TABLE products (
		id SERIAL PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		category VARCHAR(50) NOT NULL,
		price NUMERIC(10,2) NOT NULL,
		sku VARCHAR(20) NOT NULL UNIQUE,
		description TEXT,
		in_stock BOOLEAN NOT NULL DEFAULT true,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE tags (
		id SERIAL PRIMARY KEY,
		name VARCHAR(50) NOT NULL UNIQUE
	)`,
	`CREATE TABLE orders (
		id SERIAL PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id),
		status VARCHAR(20) NOT NULL,
		total NUMERIC(10,2) NOT NULL,
		notes TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	)`,
	`CREATE TABLE order_items (
		order_id INTEGER NOT NULL REFERENCES orders(id),
		product_id INTEGER NOT NULL REFERENCES products(id),
		quantity INTEGER NOT NULL,
		unit_price NUMERIC(10,2) NOT NULL,
		PRIMARY KEY (order_id, product_id)
	)`,
	`CREATE TABLE product_tags (
		product_id INTEGER NOT NULL REFERENCES products(id),
		tag_id INTEGER NOT NULL REFERENCES tags(id),
		PRIMARY KEY (product_id, tag_id)
	)`,
	`CREATE SCHEMA IF NOT EXISTS analytics`,
	`CREATE TABLE analytics.events (
		id SERIAL PRIMARY KEY,
		event_type VARCHAR(50) NOT NULL,
		user_id INTEGER NOT NULL REFERENCES public.users(id),
		payload JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT NOW()
	)`,
}

// ── MySQL DDL ─────────────────────────────────────────────────────────────────

var mysqlDropStatements = []string{
	"DROP TABLE IF EXISTS order_items",
	"DROP TABLE IF EXISTS product_tags",
	"DROP TABLE IF EXISTS orders",
	"DROP TABLE IF EXISTS products",
	"DROP TABLE IF EXISTS tags",
	"DROP TABLE IF EXISTS users",
}

var mysqlCreateStatements = []string{
	`CREATE TABLE users (
		id INT AUTO_INCREMENT PRIMARY KEY,
		username VARCHAR(50) NOT NULL UNIQUE,
		email VARCHAR(100) NOT NULL UNIQUE,
		display_name VARCHAR(100),
		is_active TINYINT(1) NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NULL
	)`,
	`CREATE TABLE products (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(100) NOT NULL,
		category VARCHAR(50) NOT NULL,
		price DECIMAL(10,2) NOT NULL,
		sku VARCHAR(20) NOT NULL UNIQUE,
		description TEXT,
		in_stock TINYINT(1) NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`,
	`CREATE TABLE tags (
		id INT AUTO_INCREMENT PRIMARY KEY,
		name VARCHAR(50) NOT NULL UNIQUE
	)`,
	`CREATE TABLE orders (
		id INT AUTO_INCREMENT PRIMARY KEY,
		user_id INT NOT NULL,
		status VARCHAR(20) NOT NULL,
		total DECIMAL(10,2) NOT NULL,
		notes TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (user_id) REFERENCES users(id)
	)`,
	`CREATE TABLE order_items (
		order_id INT NOT NULL,
		product_id INT NOT NULL,
		quantity INT NOT NULL,
		unit_price DECIMAL(10,2) NOT NULL,
		PRIMARY KEY (order_id, product_id),
		FOREIGN KEY (order_id) REFERENCES orders(id),
		FOREIGN KEY (product_id) REFERENCES products(id)
	)`,
	`CREATE TABLE product_tags (
		product_id INT NOT NULL,
		tag_id INT NOT NULL,
		PRIMARY KEY (product_id, tag_id),
		FOREIGN KEY (product_id) REFERENCES products(id),
		FOREIGN KEY (tag_id) REFERENCES tags(id)
	)`,
}

// ── SQLite DDL ────────────────────────────────────────────────────────────────

var sqliteDropStatements = []string{
	"DROP TABLE IF EXISTS order_items",
	"DROP TABLE IF EXISTS product_tags",
	"DROP TABLE IF EXISTS orders",
	"DROP TABLE IF EXISTS products",
	"DROP TABLE IF EXISTS tags",
	"DROP TABLE IF EXISTS users",
}

var sqliteCreateStatements = []string{
	`CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		email TEXT NOT NULL UNIQUE,
		display_name TEXT,
		is_active INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT
	)`,
	`CREATE TABLE products (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		category TEXT NOT NULL,
		price REAL NOT NULL,
		sku TEXT NOT NULL UNIQUE,
		description TEXT,
		in_stock INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE tags (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE
	)`,
	`CREATE TABLE orders (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL REFERENCES users(id),
		status TEXT NOT NULL,
		total REAL NOT NULL,
		notes TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`,
	`CREATE TABLE order_items (
		order_id INTEGER NOT NULL REFERENCES orders(id),
		product_id INTEGER NOT NULL REFERENCES products(id),
		quantity INTEGER NOT NULL,
		unit_price REAL NOT NULL,
		PRIMARY KEY (order_id, product_id)
	)`,
	`CREATE TABLE product_tags (
		product_id INTEGER NOT NULL REFERENCES products(id),
		tag_id INTEGER NOT NULL REFERENCES tags(id),
		PRIMARY KEY (product_id, tag_id)
	)`,
}
