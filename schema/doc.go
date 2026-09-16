// Package schema holds the shared blocking-report contract: the JSON Schema,
// the Go types generated from it, and golden fixtures.
//
// Four tools produce or consume documents in this format:
//
//   - ShareLock    live detection, enrichment and intervention
//   - sqltop       point-in-time blocking snapshot
//   - sql-auditor  historical blocking as one audit chapter
//   - SqlGoPace    a report when its own DDL is blocked, or blocks others
//
// The contract rules that matter, and why:
//
//   - schema_version leads every document. Additive changes bump minor;
//     any removal or retype bumps major.
//   - Three-state booleans stay three-state. is_optimized_locking_on: null
//     means "not available on this platform", which is not false. Collapsing
//     it loses real information.
//   - Absence is typed. Every omission carries a reason, never a bare null
//     and never a missing key — a consumer must be able to distinguish
//     "nothing was wrong" from "we could not look".
//   - Truncation is declared. Any capped collection carries truncated and
//     omitted_count.
//   - Units live in field names: _ms, _kb, _bytes, _us. The blocked-process
//     report gives durations in MICROseconds while wait_time is in
//     MILLIseconds; mixing them silently is the bug this rule prevents.
//
// TODO: the JSON Schema and generated types. The structure is specified in
// ShareLock's SPEC01 §12.
package schema
