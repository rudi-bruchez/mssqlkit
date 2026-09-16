# AGENTS.md

Instructions for AI agents working in this repository. Read `README.md` first
for what the project is; this file covers how to change it without breaking it.

## What this repository actually is

A **fact library**. The Go is thin; the value is in `*/data/*.yaml` — sourced,
dated SQL Server facts that four separate tools rely on to give advice.

That shapes every rule below. The worst outcome here is not a crash. It is a
plausible, confident, wrong answer: telling a DBA to use `ONLINE = ON` on an
edition that does not have it, or reporting a server as being on a cumulative
update it is not on. Code that fails is debugged in an hour; data that lies is
believed for a year.

## Hard rules

1. **Never edit a `*_gen.go` file.** It is overwritten by `go run ./codegen`.
   Edit the YAML it came from.
2. **Never hand-edit `platform/data/builds.yaml`.** It is produced by
   `codegen/cmd/fetchbuilds`. Rerun the tool.
3. **Never add a fact without a `source:` URL pointing at Microsoft Learn**, and
   never without updating `verified:`. The generator rejects a feature with no
   source; it cannot reject a *wrong* one, so the URL must be one you actually
   fetched.
4. **Never state a SQL Server fact from memory.** Use the `microsoft-learn` MCP
   tools (`microsoft_docs_search`, then `microsoft_docs_fetch` for the full
   page) and cite the page you read. The single likeliest error in this
   repository is "surely ONLINE index rebuild reached Standard edition by now" —
   it did not, confirmed in the editions tables for 2017, 2019, 2022 and 2025.
5. **Do not copy SQL from `sp_WhoIsActive`.** It is GPL v3 and this repository is
   MIT. Derive equivalent queries from Learn documentation instead.
6. **Do not lower the inclusion bar.** Something belongs here only if two or more
   tools consume it *and* divergence between copies would be a bug. Everything
   else goes in the consuming tool's `internal/`.

## Commands

From the repository root. `./...` does **not** work here — multi-module
workspace with no root module.

```bash
go run ./codegen                                   # YAML -> Go
go test ./platform/... ./ddlmatrix/...
go vet  ./platform/... ./ddlmatrix/...
go run ./codegen/cmd/fetchbuilds                   # network; review the diff
go run ./codegen && git diff --exit-code -- '*_gen.go'   # generated Go is current
```

Commit the YAML and its generated Go **in the same commit**. A commit with one
and not the other leaves the repository in a state where the test suite passes
and the data is wrong.

## Invariants that must survive your change

These are load-bearing and each one has a test. If a test in this list starts
failing because of your change, the change is wrong — do not adjust the test
until you can explain why the invariant no longer applies.

| Invariant | Why |
| --- | --- |
| An unknown `EngineEdition` resolves to `FamilyUnknown`, never to a default | A new Azure platform silently profiled as "box" produces confidently wrong capability decisions |
| `undetermined` is never conflated with `not supported` | Declining to answer and answering no are different conversations for the DBA |
| An empty generated table is an error, not a result | An empty table makes every version-gated decision downstream silently permissive |
| `ResolveBuild` returns `(build, exact, ok)` — `exact` is not optional | A build newer than the embedded table resolves to the newest *known* update; reported as exact, that is a wrong CU |
| `ddlmatrix` checks statement kind **before** dependencies | "FOREIGN KEY is never resumable" is intrinsic; reporting it as an unmet ONLINE dependency sends the DBA to fix an edition problem that would not help |
| Version floors apply to the box product only | `srv.Year` is 0 on Azure platforms and would fail every comparison |
| `Reason` stays an enum, not a boolean | Edition limit, version floor and statement-kind exclusion are three different remediations |

## SQL conventions

Every query this project or its consumers send to SQL Server carries query
hints, because the tool must not become part of the incident it is diagnosing:

- `OPTION (RECOMPILE, MAXDOP 1)` for **one-off** queries — keeps them out of the
  plan cache entirely.
- `OPTION (MAXDOP 1)` alone for **repeated** queries — `RECOMPILE` on a polling
  query buys a costly recompilation every interval.

`MAXDOP 1` is not about speed. It bounds the worker-thread footprint so a
diagnostic query cannot worsen `THREADPOOL` pressure on a server that is already
in trouble.

## When adding a dataset

Validation is the deliverable, not the YAML. For each new dataset ask: what is
the mistake this data invites, and does `codegen` fail on it? Existing examples:
duplicate EngineEdition values, a feature with no source, a `requires:` naming a
feature that does not exist, `KnownFields(true)` so a mistyped key fails the
build rather than vanishing.

Then write a test asserting the table is **populated**, not merely that it
parses.

## When the source page changes

`fetchbuilds` scrapes documentation, not an API. When it fails, the correct
response is almost never to loosen the parser. Read the page, confirm what
changed, and update `expectedHeader` or the regexes deliberately. Widening a
regex until the error goes away is how a fact library starts emitting silence
instead of facts.

The one exception already handled: a small number of very old rows publish no
release date (SQL Server 2005 RTM, `9.00.1399`). The tool counts these, warns,
and fails only if they exceed half the rows.

## Before declaring work done

- [ ] `go run ./codegen` then `git diff --exit-code -- '*_gen.go'` is clean
- [ ] `go test ./platform/... ./ddlmatrix/...` passes
- [ ] `go vet` passes on both modules
- [ ] Every new or changed fact has a `source:` you actually fetched, and
      `verified:` is today
- [ ] The README's data catalogue and Known limitations sections still describe
      reality
- [ ] Anything you knowingly left broken or approximate is written in the
      README's **Known limitations**, not only in a commit message

Do not commit or push unless asked.
