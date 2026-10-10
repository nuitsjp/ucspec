package adapters

import (
	"encoding/json"
	"github.com/nuitsjp/usecase-spec/internal/analysis"
	"github.com/nuitsjp/usecase-spec/internal/assets"
	"github.com/nuitsjp/usecase-spec/internal/compiler"
	"github.com/nuitsjp/usecase-spec/internal/project"
	"github.com/nuitsjp/usecase-spec/internal/report"
	"os"
	"path/filepath"
	"testing"
)

// These integration tests require the REAL downloaded modules. They must run in
// Windows CI before release; they are not counted among offline core tests.
func TestYAMLAndSchema(t *testing.T) {
	c, e := Codec(assets.Files)
	if e != nil {
		t.Fatal(e)
	}
	data, e := c.Decode([]byte("schemaVersion: '1'\nscreens:\n  a:\n    label: 検索\n"), ".yaml")
	if e != nil {
		t.Fatal(e)
	}
	var doc any
	if e = json.Unmarshal(data, &doc); e != nil {
		t.Fatal(e)
	}
	if e = c.Validate("document", doc); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{"a: 1\na: 2", "a: &a 1\nb: *a", "a: 1\n---\nb: 2", "1: text"} {
		if _, e = c.Decode([]byte(bad), ".yaml"); e == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if c.Validate("document", map[string]any{"schemaVersion": "1", "screens": map[string]any{"x": map[string]any{"typo": true}}}) == nil {
		t.Fatal("schema accepted invalid screen")
	}
}
func TestActualELKSampleBuild(t *testing.T) {
	root := t.TempDir()
	if _, e := project.Init(assets.Files, project.Options{Root: root, Name: "ELK Test", Version: "0.1.0", Repository: "nuitsjp/usecase-spec"}); e != nil {
		t.Fatal(e)
	}
	c, e := Codec(assets.Files)
	if e != nil {
		t.Fatal(e)
	}
	s, e := compiler.Load(filepath.Join(root, "spec/project.json"), c)
	if e != nil {
		t.Fatal(e)
	}
	r := analysis.Analyze(s, "integration")
	if r.Status != "verified" {
		t.Fatalf("analysis status %s", r.Status)
	}
	b, e := report.Build(s, r, LayoutJSON, assets.Files)
	if e != nil {
		t.Fatal(e)
	}
	if len(b) < 1000 {
		t.Fatal("empty report")
	}
	if e = os.WriteFile(filepath.Join(root, "index.html"), b, 0600); e != nil {
		t.Fatal(e)
	}
}
