# Loar

Loar is a knowledge runtime.

It ingests information, structures knowledge, preserves relationships, and retrieves evidence — so the right context is available when you need it.

---

## What it is

Most systems store data. Loar focuses on retrieving understanding.

Instead of asking an LLM to remember things, you give Loar your information. When you ask a question, Loar finds the relevant evidence first. The LLM — if you use one at all — gets context it can trust.

Loar is infrastructure, not an assistant. It works without an LLM entirely.

---

## What it does

```
Information → Loar → Evidence → Decision
```

- Ingest anything: JSON files, directories, URLs, stdin
- Store observations with full temporal context
- Retrieve by asking questions in plain language
- Return a structured context package — timeline, evidence, contradictions

---

## Getting started

### 1. Install

```sh
git clone https://github.com/balpal4495/loar
cd loar
make install
```

### 2. Start a database

Loar uses PostgreSQL. If you have Docker:

```sh
make db-up
```

Or use an existing Postgres instance.

### 3. Configure

```sh
loar setup
```

Detects your Postgres instance, creates the `loar` database user, and writes `~/.config/loar/config.toml`. Run once per machine.

### 4. Create a project

```sh
cd ~/your-project
loar project use
```

Uses the directory name as the project name. Creates a dedicated `loar-<name>` database and writes `.loar/project.toml`.

Or specify a name:

```sh
loar project use tierone
```

### 5. Ingest

```sh
loar ingest transfers.json
loar ingest ./data/
loar ingest ./data/ --recursive
cat feed.ndjson | loar ingest
loar ingest https://example.com/feed.json
```

Supports JSON objects, JSON arrays, NDJSON, and `.jsonl`. Handles pretty-printed and minified files. Auto-repairs truncated JSON.

### 6. Ask questions

```sh
loar query "What decisions were made about the scoring model?"
loar explain "What changed in phase 4?"
loar "Why was the fundamentals provider switched?"
```

---

## Commands

| Command | Description |
|---|---|
| `loar setup` | First-run configuration — detects Postgres, creates user, writes config |
| `loar setup --reset` | Reconfigure from scratch |
| `loar version` | Print version, commit, and build date |
| `loar project use [name]` | Associate current directory with a project (defaults to dir name) |
| `loar project list` | List all projects on the configured Postgres server |
| `loar project delete <name>` | Drop the project database and remove local config |
| `loar ingest [file\|dir\|url\|-]` | Ingest data into the current project |
| `loar query <question>` | Query the knowledge store |
| `loar explain <question>` | Retrieve evidence and produce a narrative explanation |

---

## How retrieval works

```
Question
  ↓ Intent detection
  ↓ Entity resolution
  ↓ Relationship traversal
  ↓ Evidence gathering
  ↓ Context package
```

Loar does not pass your question directly to an LLM. It finds the relevant observations first, then returns them as structured output you can use however you want.

---

## Configuration

**Global** (`~/.config/loar/config.toml`) — written by `loar setup`:
```toml
postgres_host     = "localhost"
postgres_port     = 5432
postgres_user     = "loar"
postgres_password = "loar"
```

**Per-project** (`.loar/project.toml`) — written by `loar project use`:
```toml
project     = "tierone"
database_url = "postgres://loar:loar@localhost:5432/loar-tierone?sslmode=disable"
```

`LOAR_DATABASE_URL` environment variable overrides the project DSN — useful for CI.

---

## Development

```sh
make build       # compile to ./loar
make install     # build and copy to /opt/homebrew/bin/loar
make reinstall   # clean rebuild and reinstall
make uninstall   # remove from bin
make test        # run tests
make db-up       # start Postgres in Docker
make db-down     # stop Postgres
```

---

## Design

See [Design-spec.md](Design-spec.md) for the full design specification.

Loar is intentionally incomplete. The concepts described are hypotheses validated through usage. When evidence contradicts the design, the design changes.

---

## License

[MIT](LICENSE)
