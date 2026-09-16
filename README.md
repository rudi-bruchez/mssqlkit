# mssqlkit

Shared foundations for a family of Go tools that work on SQL Server locking
and deployment:

| Tool | Role |
| --- | --- |
| [ShareLock](https://github.com/rudi-bruchez/ShareLock) | Detects, explains and optionally resolves blocking, unattended |
| [sqltop](https://github.com/rudi-bruchez/sqltop) | Point-in-time view of sessions and blocking |
| [sql-auditor](https://github.com/rudi-bruchez/sql-auditor) | Broad audit; blocking history is one chapter |
| [SqlGoPace](https://github.com/rudi-bruchez/SqlGoPace) | Resilient DDL task runner |

Each tool lives in its own repository and imports what it needs from here.

## Modules

This repository is **multi-module**: each directory below is an independently
versioned Go module, so a consumer takes only what it uses and is not forced
to absorb churn from the rest.

| Module | Contents | Consumers |
| --- | --- | --- |
| `schema/` | The blocking-report contract: JSON Schema, Go types, fixtures | all four |
| `platform/` | `SERVERPROPERTY('EngineEdition')` mapping, capability probe, build list | all four |
| `ddlmatrix/` | `ONLINE` / `WAIT_AT_LOW_PRIORITY` / `RESUMABLE` supportability | ShareLock, SqlGoPace |
| `client/` | Driver, auth, self-protection settings, query hints, `wait_resource`, XE parsing | 2–4 |

```go
import (
    "github.com/rudi-bruchez/mssqlkit/platform"
    "github.com/rudi-bruchez/mssqlkit/ddlmatrix"
)

srv, err := platform.Probe(ctx, db)
v := ddlmatrix.Supports(srv, ddlmatrix.FeatureOnlineIndexOperations, "alter_index_rebuild")
if !v.Supported {
    log.Printf("%s (%s)", v.Detail, v.Reason)
    // e.g. "Online index create and rebuild requires an Enterprise-class
    //       edition; this instance reports Standard (edition)"
}
```

Tags are per module: `platform/v0.1.0`, `ddlmatrix/v0.1.0`, …

## What belongs here

> Something enters `mssqlkit` when it is consumed by **two or more** tools
> **and** divergence between copies would be a bug.

Everything else stays in the consuming tool's `internal/`, where Go enforces
the boundary rather than discipline. This rule exists because a shared
repository with a lower bar becomes a dumping ground, and then nobody trusts
what is in it.

## Facts are data, not code

`platform/` and `ddlmatrix/` hold **YAML fact files** under `data/`. Typed Go
is generated from them; the YAML is the source of truth.

Every entry carries the Microsoft Learn URL it was read from and the date it
was verified. A fact without a source is a guess someone will act on — and
these particular facts are load-bearing. Getting `ONLINE = ON` wrong means
telling a Standard-edition DBA to use a feature their server does not have.

```bash
go generate ./...                      # YAML -> Go. Offline, deterministic.
go run ./codegen/cmd/fetchbuilds       # Refresh the build list. Needs network.
```

The two are deliberately separate. `go generate` is offline and reproducible,
so CI can verify the generated Go is current without touching the network.
`fetchbuilds` scrapes a documentation page whose structure is **not a
contract**, so it is run by a human who reviews the diff — and it fails loudly
on an unexpected table header or an implausible row count rather than quietly
emitting an empty table.

### Adding or correcting a fact

1. Edit the YAML under `<module>/data/`.
2. Include `source:` (a Learn URL) and update `verified:`.
3. `go generate ./...`
4. `go test ./...`
5. Commit the YAML **and** the generated `*_gen.go` together.

The generator rejects a YAML key it does not know, rather than ignoring it, so
a typo fails the build instead of silently dropping a fact.

## Licence

MIT. Note that `sp_WhoIsActive` is GPL v3: its SQL must not be copied,
transcribed or adapted into this repository. Derive from Microsoft Learn.
