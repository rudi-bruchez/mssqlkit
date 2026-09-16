package platform

import "testing"

// The edition mapping is load-bearing for every capability decision
// downstream, so it is asserted explicitly rather than trusted.
func TestEngineEditionMapping(t *testing.T) {
	cases := []struct {
		edition         EngineEdition
		family          Family
		enterpriseClass bool
	}{
		{EditionStandard, FamilyBox, false},
		{EditionEnterprise, FamilyBox, true},
		{EditionExpress, FamilyBox, false},
		{EditionAzureSQLDatabase, FamilyAzureSQLDatabase, true},
		{EditionAzureSQLManagedInstance, FamilyManagedInstance, true},
	}
	for _, c := range cases {
		info, ok := Lookup(c.edition)
		if !ok {
			t.Fatalf("EngineEdition %d not found in table", c.edition)
		}
		if info.Family != c.family {
			t.Errorf("EngineEdition %d: family = %q, want %q", c.edition, info.Family, c.family)
		}
		if info.EnterpriseClass != c.enterpriseClass {
			t.Errorf("EngineEdition %d: EnterpriseClass = %v, want %v",
				c.edition, info.EnterpriseClass, c.enterpriseClass)
		}
	}
}

// Standard Developer (SQL Server 2025+) reports EngineEdition 2, so Developer
// does NOT imply Enterprise-class. This is the trap the matrix exists to
// avoid: recommending ONLINE = ON on a Developer edition that cannot do it.
func TestStandardCoversStandardDeveloper(t *testing.T) {
	info, _ := Lookup(EditionStandard)
	var found bool
	for _, c := range info.Covers {
		if c == "Standard Developer" {
			found = true
		}
	}
	if !found {
		t.Errorf("EditionStandard.Covers = %v, want it to include Standard Developer", info.Covers)
	}
	if info.EnterpriseClass {
		t.Error("EditionStandard must not be Enterprise-class")
	}
}

// An EngineEdition the table does not know must be reported as unknown, never
// defaulted to a family. A new Azure platform silently profiled as "box"
// would produce confidently wrong capability decisions.
func TestUnknownEditionIsNotDefaulted(t *testing.T) {
	if _, ok := Lookup(EngineEdition(9999)); ok {
		t.Error("Lookup(9999) reported ok for an unknown edition")
	}
}

func TestMajorAndYear(t *testing.T) {
	cases := []struct {
		version string
		major   string
		year    int
	}{
		{"17.0.5005.3", "17.0", 2025},
		{"16.0.4295.3", "16.0", 2022},
		{"15.0.4430.1", "15.0", 2019},
		{"14.0.3456.2", "14.0", 2017},
		{"13.0.5888.11", "13.0", 2016},
		{"not a version", "", 0},
	}
	for _, c := range cases {
		if got := MajorOf(c.version); got != c.major {
			t.Errorf("MajorOf(%q) = %q, want %q", c.version, got, c.major)
		}
		if got := YearOf(c.version); got != c.year {
			t.Errorf("YearOf(%q) = %d, want %d", c.version, got, c.year)
		}
	}
}

func TestResolveBuildExact(t *testing.T) {
	// A build published on the source page must resolve exactly.
	b, exact, ok := ResolveBuild("16.0.4295.3")
	if !ok {
		t.Fatal("ResolveBuild(16.0.4295.3) not found")
	}
	if !exact {
		t.Error("a published build must resolve as exact")
	}
	if b.Build != "16.0.4295.3" {
		t.Errorf("Build = %q, want exact match", b.Build)
	}
	if b.Year != 2022 {
		t.Errorf("Year = %d, want 2022", b.Year)
	}
	if b.Update == "" {
		t.Error("Update is empty; the update column did not parse")
	}
}

// A build newer than the embedded table must resolve to the most recent known
// update below it, not to nothing — but the caller can tell it was inexact
// because Build differs from the queried version.
func TestResolveBuildInexact(t *testing.T) {
	b, exact, ok := ResolveBuild("16.0.9999.9")
	if !ok {
		t.Fatal("ResolveBuild fell through entirely for a future 2022 build")
	}
	if exact {
		t.Error("an unpublished build reported as exact - callers would report a wrong CU")
	}
	if b.Build == "16.0.9999.9" {
		t.Error("an unpublished build returned itself")
	}
	if b.Year != 2022 {
		t.Errorf("Year = %d, want 2022", b.Year)
	}
}

// The generated build table must be populated. An empty table would make
// every version-gated decision downstream silently permissive.
func TestBuildTableIsPopulated(t *testing.T) {
	const floor = 200
	if len(builds) < floor {
		t.Fatalf("builds table has %d entries, want >= %d - did fetchbuilds run?", len(builds), floor)
	}
	if len(releases) < 5 {
		t.Fatalf("releases table has %d entries, want >= 5", len(releases))
	}
}

// A build with more components than the table publishes must not compare equal
// to its four-part prefix, or it resolves to the wrong update level.
func TestResolveBuildExtraComponents(t *testing.T) {
	_, exact, ok := ResolveBuild("16.0.4295.3.1")
	if ok && exact {
		t.Error("16.0.4295.3.1 reported as an exact match for 16.0.4295.3")
	}
}
