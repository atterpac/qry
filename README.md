# qry

A terminal-based database client with vim-style navigation, inline editing, and multi-engine support.

Supports **PostgreSQL**, **MySQL**, and **SQLite**.

## Features

- **Schema explorer** with master-detail layout, table/view listing, and column info
- **Table data viewer** with pagination, sorting, filtering, and inline cell editing
- **Query editor** with context-aware autocomplete and split results pane
- **Foreign key traversal** across tables via `gd`
- **Export** to CSV, JSON, or SQL INSERT (clipboard or file)
- **Neovim LSP integration** for autocomplete when using `$EDITOR`
- **Profiles** for managing multiple database connections
- **Bookmarks**, **jump list**, and **query history** for fast navigation
- **Saved queries** per profile
- **Custom commands** with template variables
- **Theme selector** with live preview
- **Global fuzzy finder** for tables and saved queries

## Install

```sh
go install github.com/atterpac/qry/cmd/qry@latest
```

## Configuration

Config is loaded from `~/.config/qry/config.yaml` (or `$XDG_CONFIG_HOME/qry/config.yaml`).

```yaml
theme: "tokyonight-night"
active_profile: "local-pg"
max_history: 100

profiles:
  local-pg:
    engine: postgres
    dsn: "postgres://user:pass@localhost:5432/mydb"
    saved_queries:
      - name: "active users"
        query: "SELECT * FROM users WHERE active = true"
    commands:
      vacuum:
        cmd: "VACUUM ANALYZE"
        confirm: true

  staging-mysql:
    engine: mysql
    dsn: "user:pass@tcp(localhost:3306)/mydb"

  local-sqlite:
    engine: sqlite
    path: "~/data/app.db"

bookmarks:
  - type: table
    name: users
    schema: public

commands:
  cleanup:
    cmd: "DELETE FROM logs WHERE created_at < NOW() - INTERVAL '30 days'"
    confirm: true
```

Environment variables are expanded in DSN and path fields (`$DB_PASSWORD`, etc).

## CLI Flags

```
qry                           # launch with active profile
qry -profile staging-mysql    # connect to a specific profile
qry -dsn postgres://...       # override DSN directly
qry -path ./test.db           # open a SQLite file
qry -engine sqlite            # force engine type
qry -theme catppuccin         # override theme
qry -version                  # print version
```

## Keybindings

### Global

| Key | Action |
|-----|--------|
| `q` | Quit / pop view |
| `Esc` | Dismiss modal / pop view |
| `?` | Help |
| `T` | Theme selector |
| `P` | Profile selector |
| `:` | Command bar |
| `'` | Jump to bookmark |
| `Ctrl+P` | Global fuzzy finder |
| `Ctrl+O` | Jump back |
| `Ctrl+I` | Jump forward |

### Schema Explorer

| Key | Action |
|-----|--------|
| `Enter` | Open table |
| `/` | Search tables |
| `e` | Open query editor |
| `s` | Cycle schema |
| `r` | Refresh |
| `i` | Connection info |
| `g` / `G` | First / last |
| `j` / `k` | Navigate |

### Table Data

| Key | Action |
|-----|--------|
| `Enter` | Edit cell |
| `Ctrl+S` | Submit pending changes |
| `u` | Undo cell edit |
| `U` | Undo all edits |
| `.` | Repeat last edit |
| `/` | Search / filter (`col:value` syntax) |
| `n` / `N` | Next / previous page |
| `gd` | Follow foreign key |
| `dd` | Delete row |
| `o` / `O` | Insert row |
| `yy` | Yank row |
| `yp` | Paste yanked row as insert |
| `Ctrl+Y` | Copy row as INSERT to clipboard |
| `Ctrl+E` | Export (CSV, JSON, SQL) |
| `Shift+K` | Show table schema |
| `W` | View cell detail |
| `m` | Bookmark table |
| `g` / `G` | First / last row |
| `j` / `k` | Navigate |

### Query Editor

| Key | Action |
|-----|--------|
| `Ctrl+R` | Execute query |
| `Ctrl+S` | Save query |
| `Ctrl+E` | Open in `$EDITOR` |
| `Tab` | Accept autocomplete / switch pane |
| `Shift+K` | Schema info overlay |

## Command Bar

Press `:` to open the command bar. Tab-completion is supported.

| Command | Action |
|---------|--------|
| `tables` | Schema explorer |
| `editor [sql]` | Query editor (optionally with initial SQL) |
| `e [sql]` | Alias for `editor` |
| `info` | Connection info |
| `databases` | List databases |
| `queries` | Saved queries |
| `history` | Query history |
| `run <sql>` | Execute SQL |
| `table <name>` | Open table data |
| `sort <col> [desc]` | Sort current table |
| `count` | Count rows in current table |
| `describe` | Show table schema |
| `profile` | Switch profile |
| `quit` | Exit |

Custom commands defined in your config are also available here.

## Custom Commands

Define reusable commands globally or per-profile:

```yaml
commands:
  refresh:
    description: "Refresh materialized views"
    cmd: "REFRESH MATERIALIZED VIEW {table}"
    confirm: true
```

Template variables: `{table}`, `{schema}`, `{database}`, `{engine}`, `{query}`

## Engine Support

| Feature | PostgreSQL | MySQL | SQLite |
|---------|-----------|-------|--------|
| Multiple schemas | Yes | No | No |
| Multiple databases | Yes | Yes | No |
| Foreign keys | Yes | Yes | Yes |
| RETURNING clause | Yes | No | No |
| Inline editing | Yes | Yes | Yes |
| Autocomplete | Yes | Yes | Yes |

## Neovim Integration

When `$EDITOR` contains `nvim`, qry automatically starts an LSP server and configures neovim with schema-aware autocomplete for the current connection. No setup required.
