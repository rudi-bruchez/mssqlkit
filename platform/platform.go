// Package platform identifies which SQL Server platform a connection is
// talking to, and what that platform can do.
//
// It exists because the three supported platforms differ in which data
// sources exist at all, not merely in degree. Code that assumes the
// on-premises surface and degrades by catching errors produces reports that
// are quietly wrong; code that probes first can state what it could not
// collect, and why.
//
// The facts in this package are generated from platform/data/*.yaml. Edit the
// YAML, not the generated Go.
package platform

//go:generate go run ../codegen

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// Server is the result of probing a connection.
type Server struct {
	EngineEdition EngineEdition
	Edition       EditionInfo
	// EditionString is SERVERPROPERTY('Edition'). Note that it returns
	// "SQL Azure" for BOTH Azure SQL Database and Managed Instance, so never
	// branch on it — branch on EngineEdition.
	EditionString string
	ProductVersion string
	ProductLevel   string
	ServerName     string
	MachineName    string
	// UpdatePolicy is meaningful only on Managed Instance, where it gates
	// features available under the Always-up-to-date policy.
	UpdatePolicy UpdatePolicy
	// Year is the SQL Server release year derived from ProductVersion
	// (2022, 2025, ...). Zero on Azure platforms, which have no release year.
	Year int
	// Build is the matching row from the published build list, when one is
	// known. BuildOk is false when no row matched at all.
	Build   Build
	BuildOk bool
	// BuildExact distinguishes an exact hit from the nearest lower build.
	// Callers that report "this server is on CU27" MUST check it: a build
	// newer than the embedded table resolves to the most recent known
	// update, which is a plausible and wrong answer if taken as exact.
	BuildExact bool
}

// IsBox reports whether this is the box product (on-premises, VM, container).
func (s Server) IsBox() bool { return s.Edition.Family == FamilyBox }

// IsAzure reports whether this is a managed Azure SQL platform.
func (s Server) IsAzure() bool {
	return s.Edition.Family == FamilyAzureSQLDatabase || s.Edition.Family == FamilyManagedInstance
}

// EnterpriseClass reports whether the server carries the Enterprise feature
// set. On Azure SQL Database and Managed Instance there is no edition gate,
// so this is true. On the box product it is true only for EngineEdition 3.
//
// Beware: "Standard Developer" (SQL Server 2025 and later) reports
// EditionStandard, so a Developer edition is not necessarily Enterprise-class.
func (s Server) EnterpriseClass() bool { return s.Edition.EnterpriseClass }

const probeQuery = `
SELECT
    CAST(SERVERPROPERTY('EngineEdition')     AS int),
    CAST(SERVERPROPERTY('Edition')           AS nvarchar(128)),
    CAST(SERVERPROPERTY('ProductVersion')    AS nvarchar(128)),
    CAST(SERVERPROPERTY('ProductLevel')      AS nvarchar(128)),
    CAST(SERVERPROPERTY('ServerName')        AS nvarchar(128)),
    CAST(SERVERPROPERTY('MachineName')       AS nvarchar(128)),
    CAST(SERVERPROPERTY('ProductUpdateType') AS nvarchar(128))
OPTION (RECOMPILE, MAXDOP 1);`

// Probe identifies the server behind db.
//
// It is safe to call on any supported platform: SERVERPROPERTY returns NULL
// for properties that do not apply rather than raising.
func Probe(ctx context.Context, db *sql.DB) (Server, error) {
	var (
		s          Server
		engine     sql.NullInt64
		edition    sql.NullString
		version    sql.NullString
		level      sql.NullString
		serverName sql.NullString
		machine    sql.NullString
		updateType sql.NullString
	)
	row := db.QueryRowContext(ctx, probeQuery)
	if err := row.Scan(&engine, &edition, &version, &level, &serverName, &machine, &updateType); err != nil {
		return Server{}, fmt.Errorf("probe: %w", err)
	}
	if !engine.Valid {
		return Server{}, fmt.Errorf("probe: SERVERPROPERTY('EngineEdition') returned NULL")
	}

	s.EngineEdition = EngineEdition(engine.Int64)
	s.EditionString = edition.String
	s.ProductVersion = version.String
	s.ProductLevel = level.String
	s.ServerName = serverName.String
	s.MachineName = machine.String
	s.UpdatePolicy = UpdatePolicy(updateType.String)

	info, ok := Lookup(s.EngineEdition)
	if !ok {
		// An unknown EngineEdition is reported as unknown, never defaulted.
		// A new Azure platform silently profiled as "box" would be worse
		// than an explicit failure to profile.
		info = EditionInfo{
			Value:  s.EngineEdition,
			Name:   fmt.Sprintf("unknown EngineEdition %d", s.EngineEdition),
			Family: FamilyUnknown,
		}
	}
	s.Edition = info
	s.Year = YearOf(s.ProductVersion)
	s.Build, s.BuildExact, s.BuildOk = ResolveBuild(s.ProductVersion)
	return s, nil
}

// MajorOf returns the "major.minor" prefix of a product version string:
// "16.0.4295.3" yields "16.0". It returns "" when the input is not a version.
func MajorOf(productVersion string) string {
	parts := strings.SplitN(strings.TrimSpace(productVersion), ".", 3)
	if len(parts) < 2 {
		return ""
	}
	if _, err := strconv.Atoi(parts[0]); err != nil {
		return ""
	}
	if _, err := strconv.Atoi(parts[1]); err != nil {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// YearOf maps a product version to its SQL Server release year:
// "16.0.4295.3" yields 2022. It returns 0 for an unknown major version, which
// includes every Azure platform (they report engine versions that do not
// correspond to a box release).
func YearOf(productVersion string) int {
	r, ok := releases[MajorOf(productVersion)]
	if !ok {
		return 0
	}
	return r.Year
}

// ProductOf maps a product version to its product name:
// "16.0.4295.3" yields "SQL Server 2022".
func ProductOf(productVersion string) (string, bool) {
	r, ok := releases[MajorOf(productVersion)]
	if !ok {
		return "", false
	}
	return r.Product, true
}

// ResolveBuild finds the published build entry for a product version.
//
// It returns (build, exact, ok). An exact match sets exact=true. Failing that,
// the highest published build below the given one within the same major
// version is returned with exact=false — that is the update level the server
// is at least at.
//
// The three-value signature is deliberate. A build newer than the embedded
// table resolves to the most recent KNOWN update, which is a plausible answer
// and a wrong one if reported as the installed CU. A two-value form invites
// exactly that mistake, because the natural `if b, ok := ...; ok` reads as
// "we know what this is".
func ResolveBuild(productVersion string) (Build, bool, bool) {
	major := MajorOf(productVersion)
	if major == "" {
		return Build{}, false, false
	}
	want, ok := parseBuild(productVersion)
	if !ok {
		return Build{}, false, false
	}
	var best Build
	var bestOk bool
	var bestParsed []int
	for _, b := range builds {
		if MajorOf(b.Build) != major {
			continue
		}
		got, ok := parseBuild(b.Build)
		if !ok {
			continue
		}
		switch compareBuild(got, want) {
		case 0:
			return b, true, true
		case -1:
			if !bestOk || compareBuild(got, bestParsed) > 0 {
				best, bestParsed, bestOk = b, got, true
			}
		}
	}
	return best, false, bestOk
}

func parseBuild(s string) ([]int, bool) {
	parts := strings.Split(strings.TrimSpace(s), ".")
	if len(parts) < 2 {
		return nil, false
	}
	out := make([]int, 0, 4)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	for len(out) < 4 {
		out = append(out, 0)
	}
	return out, true
}

func compareBuild(a, b []int) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	// parseBuild pads to four components but does not cap them, so the
	// slices can differ in length. Without this, "16.0.4295.3.1" would
	// compare equal to "16.0.4295.3" and resolve to the wrong update level.
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}
