// Package ddlmatrix answers one question: can THIS server run THIS DDL
// statement with THIS concurrency option?
//
// Two tools ask it in opposite directions. SqlGoPace asks before composing a
// statement, to choose options the server actually supports. ShareLock asks
// after an incident, to judge whether an outage was avoidable — and that
// distinction matters, because "you should have used ONLINE = ON" is not
// advice on Standard edition, it is noise that teaches the DBA to ignore the
// tool.
//
// The facts are generated from ddlmatrix/data/features.yaml. Edit the YAML,
// not the generated Go.
package ddlmatrix

//go:generate go run ../codegen

import (
	"fmt"

	"github.com/rudi-bruchez/mssqlkit/platform"
)

// Reason explains why a feature is unavailable. It is an enum rather than a
// boolean because the categories are different conversations: an edition
// limit is a licensing decision, a version floor is an upgrade, an index-type
// exclusion is a schema change. Collapsing them into "not supported" throws
// away the only part the DBA can act on.
type Reason string

const (
	ReasonNone           Reason = ""
	ReasonEdition        Reason = "edition"
	ReasonVersionFloor   Reason = "version_floor"
	ReasonStatementKind  Reason = "statement_kind"
	ReasonPlatform       Reason = "platform"
	ReasonDependencyUnmet Reason = "dependency_unmet"
	// ReasonUndetermined is returned when the server could not be profiled.
	// It is never conflated with "not supported": declining to answer is a
	// different thing from answering no.
	ReasonUndetermined Reason = "undetermined"
)

// Verdict is the answer for one (server, feature, statement) triple.
type Verdict struct {
	Feature   Feature
	Statement StatementKind
	Supported bool
	Reason    Reason
	// Detail is a human-readable explanation, safe to put in a report.
	Detail string
	// Since names the version floor when one applies.
	Since int
	// ExcludedWhen lists object-level conditions the caller must still check
	// itself: this package knows the platform, not the index. A verdict of
	// Supported=true means "the platform allows it", not "this index allows
	// it".
	ExcludedWhen []string
	// Notes carries usage constraints that a caller composing the statement
	// must honour even when Supported is true — for WAIT_AT_LOW_PRIORITY,
	// that ABORT_AFTER_WAIT = SELF is illegal with MAX_DURATION = 0 and that
	// BLOCKERS needs ALTER ANY CONNECTION. Supported answers "may I", never
	// "how".
	Notes []string
	// Source is the Learn URL backing this answer.
	Source string
}

// Supports reports whether srv can run stmt with feature f.
//
// It evaluates, in order: platform family, edition gate, statement-kind
// support, version floor, then declared dependencies. Dependencies are
// evaluated last but reported first when unmet, because RESUMABLE without
// ONLINE has two independent blockers and naming only one produces advice
// that fails on the second try.
func Supports(srv platform.Server, f Feature, stmt StatementKind) Verdict {
	info, ok := Lookup(f)
	if !ok {
		return Verdict{Feature: f, Statement: stmt, Reason: ReasonUndetermined,
			Detail: fmt.Sprintf("unknown feature %q", f)}
	}
	v := Verdict{Feature: f, Statement: stmt, Source: info.Source,
		ExcludedWhen: info.ExcludedWhen, Notes: info.Notes}

	// An unprofiled server yields "undetermined", never a guess. A matrix
	// that guesses the platform gives confidently wrong advice.
	if srv.Edition.Family == platform.FamilyUnknown || srv.Edition.Family == "" {
		v.Reason = ReasonUndetermined
		v.Detail = "server platform could not be determined"
		return v
	}

	// Statement-kind support is checked BEFORE anything server-dependent,
	// because it is an intrinsic property of the feature. "FOREIGN KEY is
	// never resumable" is true on every server; reporting it as an unmet
	// ONLINE dependency would send the DBA to fix an edition problem that
	// would not help.
	var ss *StatementSupport
	for i := range info.Statements {
		if info.Statements[i].Kind == stmt {
			ss = &info.Statements[i]
			break
		}
	}
	if ss == nil {
		v.Reason = ReasonStatementKind
		v.Detail = fmt.Sprintf("%s does not apply to %s", info.Name, stmt)
		return v
	}
	if !ss.Supported {
		v.Reason = ReasonStatementKind
		v.Detail = fmt.Sprintf("%s is not supported for %s", info.Name, stmt)
		if ss.Note != "" {
			v.Detail += " (" + ss.Note + ")"
		}
		return v
	}
	v.Since = ss.MinVersion

	// Then dependencies: report the root blocker, not a downstream symptom.
	for _, dep := range info.Requires {
		if dv := Supports(srv, dep, stmt); !dv.Supported {
			v.Reason = ReasonDependencyUnmet
			v.Detail = fmt.Sprintf("%s requires %s, which is unavailable here: %s",
				info.Name, dep, dv.Detail)
			return v
		}
	}

	switch srv.Edition.Family {
	case platform.FamilyAzureSQLDatabase:
		if !info.AzureSQLDatabase {
			v.Reason = ReasonPlatform
			v.Detail = fmt.Sprintf("%s is not available on Azure SQL Database", info.Name)
			return v
		}
	case platform.FamilyManagedInstance:
		if !info.ManagedInstance {
			v.Reason = ReasonPlatform
			v.Detail = fmt.Sprintf("%s is not available on Azure SQL Managed Instance", info.Name)
			return v
		}
	case platform.FamilyBox:
		if info.BoxRequiresEnterprise && !srv.EnterpriseClass() {
			v.Reason = ReasonEdition
			v.Detail = fmt.Sprintf(
				"%s requires an Enterprise-class edition; this instance reports %s",
				info.Name, srv.Edition.Name)
			return v
		}
	default:
		v.Reason = ReasonPlatform
		v.Detail = fmt.Sprintf("%s is not supported on %s", info.Name, srv.Edition.Name)
		return v
	}

	// Version floors apply to the box product only. Azure SQL Database and
	// Managed Instance track the latest engine, so a release-year floor is
	// meaningless there — and srv.Year is 0 on those platforms, which would
	// otherwise fail every comparison.
	if srv.Edition.Family == platform.FamilyBox && ss.MinVersion > 0 {
		if srv.Year == 0 {
			v.Reason = ReasonUndetermined
			v.Detail = fmt.Sprintf(
				"cannot determine the SQL Server release year from version %q", srv.ProductVersion)
			return v
		}
		if srv.Year < ss.MinVersion {
			v.Reason = ReasonVersionFloor
			v.Detail = fmt.Sprintf("%s for %s requires SQL Server %d or later; this instance is SQL Server %d",
				info.Name, stmt, ss.MinVersion, srv.Year)
			return v
		}
	}

	v.Supported = true
	v.Detail = fmt.Sprintf("%s is available for %s here", info.Name, stmt)
	if ss.Note != "" {
		v.Detail += " (" + ss.Note + ")"
	}
	return v
}
