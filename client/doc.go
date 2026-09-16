// Package client holds the shared SQL Server connection and parsing helpers:
// DSN handling, Microsoft Entra ID authentication, the self-protection
// session settings, the mandatory query hints, wait_resource translation, and
// Extended Events payload parsing.
//
// # Self-protection
//
// Every connection these tools open must run:
//
//	SET TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
//	SET LOCK_TIMEOUT 1000;
//	SET DEADLOCK_PRIORITY LOW;
//
// READ UNCOMMITTED is right here and only here. For pure DMVs it is
// irrelevant — they are materialised from engine structures and take no
// shared locks. It matters for the CATALOG views (sys.partitions,
// sys.databases, sys.objects), because a DDL statement holds exclusive locks
// on the sys.objects rows for its target table for the duration of its
// transaction. Reading uncommitted stops a monitoring tool from queueing
// behind the very DDL it is trying to describe.
//
// Its limit is the important part: it does NOT exempt the caller from Sch-S
// acquisition. No isolation level and no table hint bypasses Sch-M. If a
// schema convoy forms on a catalog object, these tools queue like everyone
// else — which is why query timeouts are mandatory on every call.
//
// # Query hints
//
// Two rules, both enforced by test in the consuming tools:
//
//  1. Every query statement carries OPTION (MAXDOP 1). Not for plan quality —
//     for worker-thread footprint. These tools run during incidents, and a
//     blocking storm often coincides with THREADPOOL pressure, where the
//     instance has run out of workers and new connections queue. A parallel
//     query costs one worker per degree of parallelism; a serial one costs
//     exactly one. It also keeps the tool from generating the CXPACKET waits
//     it is there to measure.
//
//  2. One-off queries also carry RECOMPILE, so they leave no plan in a cache
//     the tool does not own. Queries on a repeating cadence must NOT: paying
//     a compile every second to avoid one cached plan is a bad trade in CPU
//     and an actively hostile one during an incident.
//
// Where a query is ambiguous, the default is no RECOMPILE: the cost of one
// cached plan is bounded, the cost of a surprise compile loop is not.
//
// # Licensing boundary
//
// sp_WhoIsActive is GPL v3. mssqlkit is MIT. Its SQL must not be copied,
// transcribed or adapted into this module. Where a query here resembles one
// there, that is the DMV's shape, not authorship — derive from Microsoft
// Learn, never from GPL sources.
//
// TODO: implementation.
package client
