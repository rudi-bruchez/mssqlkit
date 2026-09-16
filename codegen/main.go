// Command codegen turns the YAML fact files under <module>/data into typed Go
// source under <module>.
//
// It lives in its own module so that the YAML dependency stays here: the
// generated packages are plain Go with no external imports, and a consumer of
// mssqlkit/platform inherits nothing from this tool.
//
// Run from the repository root:
//
//	go generate ./...
package main

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "codegen:", err)
		os.Exit(1)
	}
}

func run() error {
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if err := genEditions(root); err != nil {
		return fmt.Errorf("editions: %w", err)
	}
	if err := genFeatures(root); err != nil {
		return fmt.Errorf("features: %w", err)
	}
	if err := genBuilds(root); err != nil {
		return fmt.Errorf("builds: %w", err)
	}
	return nil
}

// ------------------------------------------------------------------ builds

type buildsDoc struct {
	Source   string `yaml:"source"`
	Fetched  string `yaml:"fetched"`
	Note     string `yaml:"note"`
	Releases []struct {
		Major   string `yaml:"major"`
		Product string `yaml:"product"`
		Year    int    `yaml:"year"`
		Builds  []struct {
			Build       string `yaml:"build"`
			ServicePack string `yaml:"service_pack"`
			Update      string `yaml:"update"`
			KB          string `yaml:"kb"`
			Released    string `yaml:"released"`
		} `yaml:"builds"`
	} `yaml:"releases"`
}

func genBuilds(root string) error {
	path := filepath.Join(root, "platform", "data", "builds.yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		// builds.yaml is produced by the fetchbuilds maintenance tool, which
		// needs the network. Its absence is not an error: generate the rest
		// and leave an empty table behind so the package still compiles.
		fmt.Println("codegen: platform/data/builds.yaml absent - run `go run ./codegen/cmd/fetchbuilds` to populate it")
		empty := buildsDoc{Source: "(not fetched)", Fetched: "(never)"}
		return writeGo(filepath.Join(root, "platform", "builds_gen.go"), buildsTmpl, empty)
	}
	var doc buildsDoc
	if err := readYAML(path, &doc); err != nil {
		return err
	}
	if len(doc.Releases) == 0 {
		return fmt.Errorf("%s has no releases", path)
	}
	for _, r := range doc.Releases {
		if r.Major == "" || r.Year == 0 {
			return fmt.Errorf("release %q is missing major or year", r.Product)
		}
	}
	return writeGo(filepath.Join(root, "platform", "builds_gen.go"), buildsTmpl, doc)
}

// repoRoot returns the directory containing this module, i.e. <repo>/codegen's
// parent. Resolved from the working directory so the tool works both from the
// repo root and from within codegen/.
func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		if parent := filepath.Dir(dir); parent == dir {
			return "", fmt.Errorf("go.work not found above %s", wd)
		}
	}
}

// ---------------------------------------------------------------- editions

type editionsDoc struct {
	Source         string `yaml:"source"`
	Verified       string `yaml:"verified"`
	EngineEditions []struct {
		Value           int      `yaml:"value"`
		Const           string   `yaml:"const"`
		Name            string   `yaml:"name"`
		Family          string   `yaml:"family"`
		Covers          []string `yaml:"covers"`
		EnterpriseClass bool     `yaml:"enterprise_class"`
		Obsolete        bool     `yaml:"obsolete"`
		Supported       *bool    `yaml:"supported"`
		Source          string   `yaml:"source"`
	} `yaml:"engine_editions"`
	UpdatePolicies struct {
		Source   string `yaml:"source"`
		Verified string `yaml:"verified"`
		Values   []struct {
			Value string `yaml:"value"`
			Const string `yaml:"const"`
			Name  string `yaml:"name"`
		} `yaml:"values"`
	} `yaml:"update_policies"`
}

func genEditions(root string) error {
	var doc editionsDoc
	if err := readYAML(filepath.Join(root, "platform", "data", "editions.yaml"), &doc); err != nil {
		return err
	}
	if len(doc.EngineEditions) == 0 {
		return fmt.Errorf("no engine_editions found")
	}
	seen := map[int]bool{}
	for _, e := range doc.EngineEditions {
		if e.Const == "" {
			return fmt.Errorf("engine edition %d has no const", e.Value)
		}
		if seen[e.Value] {
			return fmt.Errorf("duplicate engine edition value %d", e.Value)
		}
		seen[e.Value] = true
	}
	return writeGo(filepath.Join(root, "platform", "editions_gen.go"), editionsTmpl, doc)
}

// ---------------------------------------------------------------- features

type featuresDoc struct {
	Verified string `yaml:"verified"`
	Features []struct {
		ID                   string   `yaml:"id"`
		Name                 string   `yaml:"name"`
		Source               string   `yaml:"source"`
		Also                 string   `yaml:"also"`
		BoxRequiresEnterprise bool    `yaml:"box_requires_enterprise"`
		AzureSQLDatabase     bool     `yaml:"azure_sql_database"`
		ManagedInstance      bool     `yaml:"managed_instance"`
		Requires             []string `yaml:"requires"`
		Statements           []struct {
			Kind       string `yaml:"kind"`
			MinVersion int    `yaml:"min_version"`
			Supported  *bool  `yaml:"supported"`
			Note       string `yaml:"note"`
		} `yaml:"statements"`
		ExcludedWhen []string `yaml:"excluded_when"`
		Notes        []string `yaml:"notes"`
	} `yaml:"features"`
}

func genFeatures(root string) error {
	var doc featuresDoc
	if err := readYAML(filepath.Join(root, "ddlmatrix", "data", "features.yaml"), &doc); err != nil {
		return err
	}
	if len(doc.Features) == 0 {
		return fmt.Errorf("no features found")
	}
	ids := map[string]bool{}
	for _, f := range doc.Features {
		if f.Source == "" {
			return fmt.Errorf("feature %q has no source URL", f.ID)
		}
		ids[f.ID] = true
	}
	// A dependency that does not exist would silently never be satisfied.
	for _, f := range doc.Features {
		for _, r := range f.Requires {
			if !ids[r] {
				return fmt.Errorf("feature %q requires unknown feature %q", f.ID, r)
			}
		}
	}
	return writeGo(filepath.Join(root, "ddlmatrix", "features_gen.go"), featuresTmpl, doc)
}

// ---------------------------------------------------------------- helpers

func readYAML(path string, into any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true) // a typo in the YAML must fail the build, not vanish
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func writeGo(path, tmpl string, data any) error {
	t := template.Must(template.New("gen").Funcs(template.FuncMap{
		"quote": func(s string) string { return fmt.Sprintf("%q", s) },
		"join":  func(v []string) string { return strings.Join(v, ", ") },
		// goName turns an id like "wait_at_low_priority" into "WaitAtLowPriority"
		// so it can be used as an exported Go identifier.
		"goName": func(s string) string {
			var b strings.Builder
			for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == '-' }) {
				b.WriteString(strings.ToUpper(part[:1]))
				b.WriteString(part[1:])
			}
			return b.String()
		},
	}).Parse(tmpl))

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return err
	}
	src, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("%s: generated source does not parse: %w", path, err)
	}
	if err := os.WriteFile(path, src, 0o644); err != nil {
		return err
	}
	fmt.Println("codegen: wrote", path)
	return nil
}
