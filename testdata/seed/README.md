# Test database harness

Spin up real databases in Docker, seed them with a shared dataset, and point
`qry` at them. Used for manual testing and as living fixtures for each engine.

## Quick start

```sh
task testdb-up                 # start all containers (waits for healthchecks)
task testdb-seed               # create schema + insert seed data in each
qry --config testdata/config.yaml   # connect with any profile
```

Other tasks:

```sh
task testdb-seed -- surrealdb  # seed only one engine
task testdb-reset              # down -v, up, seed (clean slate)
task testdb-down               # stop + remove containers and volumes
```

All ports are offset (15432, 15433, 13306, 18000) so they never collide with a
locally-installed database.

## How it fits together

Three pieces, kept in sync by a shared name (e.g. `surrealdb`):

| Piece                         | File                              | Responsibility                          |
| ----------------------------- | --------------------------------- | --------------------------------------- |
| Container                     | `testdata/docker-compose.yml`     | run the server + healthcheck            |
| Seeder                        | `testdata/seed/seed_<engine>.go`  | connect, define schema, insert data     |
| App profile                   | `testdata/config.yaml`            | how `qry` connects                       |

The seed data itself (`data.go`) and the SQL DDL (`schema.go`) are shared, so
every engine populates the same `users` / `products` / `orders` story.

The seeders self-register via `init()` (see `seeder.go`), so `main.go` never
needs editing — it just loops over the registry and runs each one (optionally
filtered by CLI args).

## Adding a new engine (worked example: SurrealDB)

`seed_surreal.go` is the reference. Four edits, no plumbing:

1. **Container** — add a service to `docker-compose.yml` with an offset port and
   a `healthcheck` (so `up --wait` blocks until it's ready to seed).

2. **Seeder** — add `seed_<engine>.go` with an `init()` that calls
   `register("<engine>", func(ctx) error { ... })`. Inside:
   - connect with a retry loop (containers take a moment after "healthy"),
   - drop + (re)create the schema,
   - insert the shared `seedUsers` / `seedProducts` / `seedOrders` data.
   Read DSN/URL from env with `envOr(...)` so CI can override.

3. **Profile** — add an entry to `testdata/config.yaml` (and optionally
   `qry-test.yaml` with saved queries) using the matching engine type.

4. **Done** — `task testdb-up && task testdb-seed -- <engine>` and connect.

The `<engine>` name should be identical across the compose service, the
`register(...)` name, and the config profile. That's the only convention.
