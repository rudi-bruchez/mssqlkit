# mssqlkit

**A curated, sourced and machine-generated body of SQL Server facts**, plus the
small amount of Go needed to query it.

Most of this repository is *data*: which engine editions exist, which build is
which cumulative update, which DDL statement accepts which concurrency option
on which edition and from which version. That data is held as YAML, carries a
Microsoft Learn URL and a verification date per entry, is compiled into typed
Go, and is refreshed by deterministic tools kept under `codegen/`.

It exists because four tools need the same answers, and because two copies of a
supportability matrix drift silently — the second copy is wrong for months
before anyone notices, and by then it has been giving confident advice.

| Tool | Role |
| --- | --- |
| [ShareLock](https://github.com/rudi-bruchez/ShareLock) | Detects, explains and optionally resolves blocking, unattended |
| [sqltop](https://github.com/rudi-bruchez/sqltop) | Point-in-time view of sessions and blocking |
| [sql-auditor](https://github.com/rudi-bruchez/sql-auditor) | Broad audit; blocking history is one chapter |
| [SqlGoPace](https://github.com/rudi-bruchez/SqlGoPace) | Resilient DDL task runner |

Each tool lives in its own repository and imports what it needs from here.

## The data catalogue

| Dataset | File | Holds | Refreshed by | Network? |
| --- | --- | --- | --- | --- |
| Engine editions | `platform/data/editions.yaml` | 12 `SERVERPROPERTY('EngineEdition')` values → family, edition class, coverage | hand, against Learn | no |
| MI update policies | `platform/data/editions.yaml` | `ProductUpdateType`: `CU` vs `Continuous` | hand, against Learn | no |
| Build list | `platform/data/builds.yaml` | 668 builds across 10 releases → SP, CU, KB, release date | `codegen/cmd/fetchbuilds` | yes |
| DDL supportability | `ddlmatrix/data/features.yaml` | 3 features × 8 statement kinds → edition gate, version floor, exclusions | hand, against Learn | no |

Each dataset compiles to a `*_gen.go` in its module. **Never edit the generated
Go**; edit the YAML and regenerate.

### Why YAML and not Go directly

A fact table written as Go is a fact table nobody audits. As YAML it is
reviewable by someone who does not read Go, diffable line by line, and — most
importantly — it carries provenance next to the value:

```yaml
- value: 2
  const: Standard
  name: Standard
  family: box
  # Covers Standard, Standard Developer, Web and Business Intelligence.
  # NOTE: "Standard Developer" (SQL Server 2025+) lands here, NOT on 3.
  covers: [Standard, Standard Developer, Web, Business Intelligence]
```

That comment is the point of the dataset. Anyone can *remember* that Developer
edition carries Enterprise features; the trap is that since SQL Server 2025
there is a Standard Developer edition that does not, and it reports
EngineEdition 2. A fact without a source is a guess someone will act on.

## Commands

Run these from the repository root.

```bash
go run ./codegen                     # YAML -> Go. Offline, deterministic, idempotent.
go run ./codegen/cmd/fetchbuilds     # Refresh builds.yaml from Learn. Needs network.
go test ./platform/... ./ddlmatrix/...
go vet  ./platform/... ./ddlmatrix/...
```

> **`./...` does not work from this root.** It is a multi-module workspace with
> no module at the top, so `go test ./...` and `go generate ./...` both fail
> with *"directory prefix . does not contain modules listed in go.work"*. List
> the modules explicitly, or `cd` into one. This costs everybody five minutes
> exactly once; it is written here so it costs you none.

The `//go:generate go run ../codegen` directives in `platform` and `ddlmatrix`
are for running `go generate ./...` *inside* a single module. They are redundant
with each other — codegen always regenerates all three files — so `go run
./codegen` from the root is the canonical invocation.

### Verifying the generated Go is current

```bash
go run ./codegen && git diff --exit-code -- '*_gen.go'
```

Regeneration is idempotent, so a non-empty diff means someone edited the YAML
without regenerating, or edited the generated Go by hand.

## Generation is deterministic; fetching is not

The two are deliberately separate commands.

**`go run ./codegen`** is offline and reproducible: same YAML in, byte-identical
Go out. CI can verify it without network access, and the check above is
meaningful precisely because nothing outside the repository can change the
result.

**`fetchbuilds`** scrapes a documentation page whose structure is **not a
contract**. It is run by a human who reviews the diff. Accordingly it fails
loudly rather than degrading: it rejects an unexpected table header, a build
count below a sanity floor, too few product releases, or a majority of
unparseable release dates. A library that quietly forgot which SQL Server
versions exist would give confidently wrong answers everywhere downstream, so
an empty table is an error, never a result.

## Modules

This repository is **multi-module**: each directory is an independently
versioned Go module, so a consumer takes only what it uses and is not forced to
absorb churn from the rest.

| Module | Contents | Status | Consumers |
| --- | --- | --- | --- |
| `platform/` | Engine-edition mapping, capability probe, build resolution | implemented | all four |
| `ddlmatrix/` | `ONLINE` / `WAIT_AT_LOW_PRIORITY` / `RESUMABLE` supportability | implemented | ShareLock, SqlGoPace |
| `schema/` | The blocking-report contract: JSON Schema, Go types, fixtures | **stub** — `doc.go` only | all four |
| `client/` | Driver, auth, self-protection, query hints, `wait_resource`, XE parsing | **stub** — `doc.go` only | 2–4 |
| `codegen/` | The generator and `fetchbuilds`. Imported by nobody. | implemented | — |

`codegen` is its own module so its YAML dependency stays here: the generated
packages are plain Go with no external imports, and a consumer of
`mssqlkit/platform` inherits nothing from the build tooling.

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

### Publication order

Tags are per module: `platform/v0.1.0`, `ddlmatrix/v0.1.0`, …

`ddlmatrix/go.mod` currently carries a `replace` pointing at `../platform`. A
`replace` applies **only to the main module**, so a downstream `go get` of
`ddlmatrix` would try to fetch `platform v0.0.0` and fail. Before publishing:

1. Tag `platform/vX.Y.Z` and push the tag.
2. Bump the `require` in `ddlmatrix/go.mod` to that version.
3. Delete the `replace`.
4. Then tag `ddlmatrix/vX.Y.Z`.

## What belongs here

> Something enters `mssqlkit` when it is consumed by **two or more** tools
> **and** divergence between copies would be a bug.

Everything else stays in the consuming tool's `internal/`, where Go enforces the
boundary rather than discipline. This rule exists because a shared repository
with a lower bar becomes a dumping ground, and then nobody trusts what is in it.

Datasets that meet the bar but are not built yet: lock mode compatibility,
wait-type taxonomy (including the `LOW_PRIORITY` and optimized-locking
spellings), per-platform DMV and Extended Events availability, and the error
numbers a blocking tool must recognise.

## Adding or correcting a fact

1. Edit the YAML under `<module>/data/`.
2. Include `source:` (a Learn URL) and update `verified:`.
3. `go run ./codegen`
4. `go test ./platform/... ./ddlmatrix/...`
5. Commit the YAML **and** the generated `*_gen.go` together.

The generator decodes with `KnownFields(true)`, so a mistyped key fails the
build rather than being silently dropped. It also rejects a duplicate
EngineEdition value, a feature with no source URL, and a dependency naming a
feature that does not exist.

## Adding a new dataset

1. `<module>/data/<name>.yaml`, with `source:` and `verified:` at the top.
2. A `*Doc` struct and a `gen*` function in `codegen/main.go`, plus validation
   that fails on the mistakes that particular dataset invites.
3. A template in `codegen/templates.go`.
4. A test asserting the table is **populated**, not merely that it parses. An
   empty table makes every decision downstream silently permissive, which is the
   one failure mode a fact library must not have.
5. A row in the catalogue table above.

## Known limitations

- **`fetchbuilds` stamps `fetched:` with today's date**, so its output always
  differs from the committed file even when no build changed. You cannot use
  `git diff --exit-code` to detect a stale build list; read the `releases:` part
  of the diff.
- **KB numbers degrade silently.** `parseDate` counts and reports its failures,
  but KB extraction still returns an empty string when the regex does not match.
  A change in how KB numbers are written on the source page would empty the
  `kb:` fields without a warning. Same class of defect as the date handling, and
  it wants the same treatment — a counter and a threshold — rather than a second
  ad-hoc fix.
- **`schema/` and `client/` are stubs.** The report contract in `schema/` is the
  blocking item: all four tools depend on it.
- **Azure release years are zero.** Azure SQL Database and Managed Instance
  report engine versions with no box release year, so version floors are not
  evaluated there. `ddlmatrix` handles this explicitly; anything new that
  compares `srv.Year` must too.

## Licence

MIT. Note that `sp_WhoIsActive` is GPL v3: its SQL must not be copied,
transcribed or adapted into this repository. Derive from Microsoft Learn.
