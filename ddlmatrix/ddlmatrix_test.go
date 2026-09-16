package ddlmatrix

import (
	"testing"

	"github.com/rudi-bruchez/mssqlkit/platform"
)

func server(t *testing.T, edition platform.EngineEdition, version string) platform.Server {
	t.Helper()
	info, ok := platform.Lookup(edition)
	if !ok {
		t.Fatalf("unknown edition %d", edition)
	}
	return platform.Server{
		EngineEdition:  edition,
		Edition:        info,
		ProductVersion: version,
		Year:           platform.YearOf(version),
	}
}

// The single most valuable test here: ONLINE index operations are
// Enterprise-only on the box product, confirmed in the editions tables for
// 2017, 2019, 2022 and 2025. Recommending ONLINE = ON on Standard is the
// failure mode most likely to get the advisor switched off.
func TestOnlineIsEnterpriseOnlyOnBox(t *testing.T) {
	std := server(t, platform.EditionStandard, "16.0.4295.3")
	v := Supports(std, FeatureOnlineIndexOperations, "alter_index_rebuild")
	if v.Supported {
		t.Fatal("ONLINE reported as supported on Standard edition")
	}
	if v.Reason != ReasonEdition {
		t.Errorf("Reason = %q, want %q", v.Reason, ReasonEdition)
	}

	ent := server(t, platform.EditionEnterprise, "16.0.4295.3")
	if v := Supports(ent, FeatureOnlineIndexOperations, "alter_index_rebuild"); !v.Supported {
		t.Errorf("ONLINE not supported on Enterprise: %s", v.Detail)
	}
}

// No edition gate on the Azure platforms.
func TestOnlineOnAzureHasNoEditionGate(t *testing.T) {
	for _, e := range []platform.EngineEdition{
		platform.EditionAzureSQLDatabase,
		platform.EditionAzureSQLManagedInstance,
	} {
		srv := server(t, e, "12.0.2000.8")
		if v := Supports(srv, FeatureOnlineIndexOperations, "alter_index_rebuild"); !v.Supported {
			t.Errorf("edition %d: ONLINE not supported: %s (%s)", e, v.Detail, v.Reason)
		}
	}
}

// The version floor for WAIT_AT_LOW_PRIORITY differs BY STATEMENT KIND: 2014
// for ALTER INDEX, 2022 for CREATE INDEX. An advisor that checks only
// ">= 2014" emits syntax that fails to parse on a 2019 box.
func TestWaitAtLowPriorityFloorDiffersByStatement(t *testing.T) {
	srv2019 := server(t, platform.EditionEnterprise, "15.0.4430.1")

	if v := Supports(srv2019, FeatureWaitAtLowPriority, "alter_index_rebuild"); !v.Supported {
		t.Errorf("ALTER INDEX on 2019 should support WAIT_AT_LOW_PRIORITY: %s", v.Detail)
	}
	v := Supports(srv2019, FeatureWaitAtLowPriority, "create_index")
	if v.Supported {
		t.Error("CREATE INDEX on 2019 must NOT support WAIT_AT_LOW_PRIORITY")
	}
	if v.Reason != ReasonVersionFloor {
		t.Errorf("Reason = %q, want %q", v.Reason, ReasonVersionFloor)
	}

	srv2022 := server(t, platform.EditionEnterprise, "16.0.4295.3")
	if v := Supports(srv2022, FeatureWaitAtLowPriority, "create_index"); !v.Supported {
		t.Errorf("CREATE INDEX on 2022 should support WAIT_AT_LOW_PRIORITY: %s", v.Detail)
	}
}

// WAIT_AT_LOW_PRIORITY has no edition gate, unlike ONLINE. On Standard it is
// the one thing actually available — which is exactly what the advisor should
// recommend there instead of ONLINE.
func TestWaitAtLowPriorityHasNoEditionGate(t *testing.T) {
	std := server(t, platform.EditionStandard, "16.0.4295.3")
	if v := Supports(std, FeatureWaitAtLowPriority, "alter_index_rebuild"); !v.Supported {
		t.Errorf("WAIT_AT_LOW_PRIORITY should be available on Standard: %s (%s)", v.Detail, v.Reason)
	}
}

// RESUMABLE requires ONLINE. On Standard both are unavailable, and the
// verdict must name the dependency as the root blocker rather than reporting
// the two independently — otherwise the DBA fixes one and hits the other.
func TestResumableReportsDependencyAsRootBlocker(t *testing.T) {
	std := server(t, platform.EditionStandard, "16.0.4295.3")
	v := Supports(std, FeatureResumable, "alter_index_rebuild")
	if v.Supported {
		t.Fatal("RESUMABLE reported as supported on Standard")
	}
	if v.Reason != ReasonDependencyUnmet {
		t.Errorf("Reason = %q, want %q (detail: %s)", v.Reason, ReasonDependencyUnmet, v.Detail)
	}
}

// FOREIGN KEY constraints are never resumable, in any version or edition.
func TestForeignKeyIsNeverResumable(t *testing.T) {
	ent := server(t, platform.EditionEnterprise, "17.0.5005.3")
	v := Supports(ent, FeatureResumable, "add_constraint_foreign_key")
	if v.Supported {
		t.Fatal("ADD CONSTRAINT FOREIGN KEY reported as resumable")
	}
	if v.Reason != ReasonStatementKind {
		t.Errorf("Reason = %q, want %q", v.Reason, ReasonStatementKind)
	}
}

// An unprofiled server must yield "undetermined", never "not supported".
// Declining to answer and answering no are different things.
func TestUnknownPlatformIsUndetermined(t *testing.T) {
	var srv platform.Server // zero value: family unknown
	v := Supports(srv, FeatureOnlineIndexOperations, "alter_index_rebuild")
	if v.Supported {
		t.Fatal("unprofiled server reported as supported")
	}
	if v.Reason != ReasonUndetermined {
		t.Errorf("Reason = %q, want %q", v.Reason, ReasonUndetermined)
	}
}

// Every feature must carry the Learn URL it was verified against. A fact
// without a source is a guess someone will act on.
func TestEveryFeatureHasASource(t *testing.T) {
	for _, id := range All() {
		info, _ := Lookup(id)
		if info.Source == "" {
			t.Errorf("feature %q has no source URL", id)
		}
	}
}
