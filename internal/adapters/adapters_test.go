package adapters

import (
	"encoding/json"
	"github.com/nuitsjp/ucspec/internal/analysis"
	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/compiler"
	"github.com/nuitsjp/ucspec/internal/project"
	"github.com/nuitsjp/ucspec/internal/report"
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

func TestHierarchicalSchema(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	parse := func(raw string) any {
		t.Helper()
		var doc any
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	validDocument := `{
		"schemaVersion":"1",
		"screens":{"customer":{"label":"Customer"}},
		"states":{
			"customer":{"label":"Customer","screen":"customer","events":["close"],"initial":"viewing"},
			"viewing":{"label":"Viewing","screen":"customer","events":[],"parent":"customer"},
			"done":{"label":"Done","screen":"customer","events":[],"terminal":true}
		},
		"events":{"close":{"label":"Close","source":"user"}},
		"transitions":{"close":{"from":"customer","to":"done","event":"close","action":"Close customer"}},
		"usecases":{"closeCustomer":{"name":"Close customer","goal":"Close customer","actor":"User","starts":[{"state":"customer"}],"end":{"eq":[{"var":"$state"},"done"]},"scope":["close"],"scenarios":{}}},
		"properties":{"insideCustomer":{"kind":"reachable","description":"Customer is active","assert":{"inState":"customer"}}}
	}`
	if err := c.Validate("document", parse(validDocument)); err != nil {
		t.Fatalf("valid nested state fields rejected: %v", err)
	}
	validProject := `{"schemaVersion":"1","name":"Hierarchy","files":["model.json"],"output":"artifacts","initial":[{"state":"customer"}],"limits":{"states":100,"steps":8,"paths":1000,"combinations":10},"coverage":{"states":100,"compositeStates":100,"transitions":100,"pairs":100,"primePaths":100,"boundedPaths":100,"conditions":100,"multipleConditions":100}}`
	if err := c.Validate("project", parse(validProject)); err != nil {
		t.Fatalf("hierarchical coverage thresholds rejected: %v", err)
	}
	root := t.TempDir()
	for name, raw := range map[string]string{"project.json": validProject, "model.json": validDocument} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := compiler.Load(filepath.Join(root, "project.json"), c)
	if err != nil {
		t.Fatalf("nested specification did not load through production codec: %v", err)
	}
	for _, d := range compiler.Validate(s) {
		if d.Severity == "error" {
			t.Fatalf("valid nested specification rejected: %+v", d)
		}
	}
	for _, raw := range []string{
		`{"schemaVersion":"1","states":{"customer":{"label":"Customer","screen":"customer","events":[],"parent":3}}}`,
		`{"schemaVersion":"1","states":{"customer":{"label":"Customer","screen":"customer","events":[],"initial":[]}}}`,
		`{"schemaVersion":"1","states":{"customer":{"label":"Customer","screen":"customer","events":[],"parent":""}}}`,
		`{"schemaVersion":"1","states":{"customer":{"label":"Customer","screen":"customer","events":[],"initial":"bad state"}}}`,
		`{"schemaVersion":"1","states":{"customer":{"label":"Customer","screen":"customer","initial":"viewing"}}}`,
		`{"schemaVersion":"1","properties":{"inside":{"kind":"reachable","description":"Inside","assert":{"inState":true}}}}`,
		`{"schemaVersion":"1","properties":{"inside":{"kind":"reachable","description":"Inside","assert":{"inState":"customer","extra":true}}}}`,
	} {
		if err := c.Validate("document", parse(raw)); err == nil {
			t.Errorf("schema accepted malformed hierarchy field: %s", raw)
		}
	}
	for _, coverage := range []string{`{"states":101}`, `{"compositeStates":-1}`, `{"stateGroups":100}`} {
		doc := parse(validProject).(map[string]any)
		doc["coverage"] = parse(coverage)
		if err := c.Validate("project", doc); err == nil {
			t.Errorf("schema accepted invalid coverage: %s", coverage)
		}
	}
}

func TestActualELKSampleBuild(t *testing.T) {
	root := t.TempDir()
	if _, e := project.Init(assets.Files, project.Options{Root: root, Name: "ELK Test", Version: "0.1.0", Repository: "nuitsjp/ucspec"}); e != nil {
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

func TestSchemaRequiresScenarioPurposeAndOutcome(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"goal", "expectedOutcome"} {
		for _, value := range []any{nil, "", " \t\n", "clear intent"} {
			scenario := map[string]any{
				"name": "normal", "goal": "complete", "expectedOutcome": "completed",
				"start": map[string]any{"state": "start"}, "steps": []any{}, "expect": true,
			}
			if value == nil {
				delete(scenario, field)
			} else {
				scenario[field] = value
			}
			doc := map[string]any{"schemaVersion": "1", "regressions": map[string]any{"normal": scenario}}
			err := c.Validate("document", doc)
			if value == "clear intent" {
				if err != nil {
					t.Fatalf("valid %s rejected: %v", field, err)
				}
			} else if err == nil {
				t.Fatalf("schema accepted missing/blank %s: %#v", field, value)
			}
		}
	}
}

func TestSchemaCoverageReviews(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"valid", "states", "compositeStates", "kind", "status", "reason", "basisHash", "requirements", "duplicateRequirement", "scenario", "architectureRequirements"} {
		t.Run(change, func(t *testing.T) {
			review := map[string]any{
				"kind": "boundedPaths", "requirements": []any{"retry > finish"}, "status": "represented",
				"reason": "same recovery purpose", "scenario": "normal", "basisHash": "recorded-context",
				"architectureRequirements": []any{"preserve input while retrying"},
			}
			switch change {
			case "states", "compositeStates":
				review["kind"] = change
			case "kind":
				review["kind"] = "unknown"
			case "status":
				review["status"] = "approved"
			case "reason":
				review["reason"] = " "
			case "basisHash":
				delete(review, "basisHash")
			case "requirements":
				review["requirements"] = []any{}
			case "duplicateRequirement":
				review["requirements"] = []any{"retry", "retry"}
			case "scenario":
				delete(review, "scenario")
			case "architectureRequirements":
				review["architectureRequirements"] = []any{" "}
			}
			scenario := map[string]any{
				"name": "normal", "goal": "complete", "expectedOutcome": "completed",
				"start": map[string]any{"state": "start"}, "steps": []any{}, "expect": true,
			}
			usecase := map[string]any{
				"name": "flow", "goal": "complete", "actor": "user", "starts": []any{map[string]any{"state": "start"}},
				"end": true, "scope": []any{"finish"}, "scenarios": map[string]any{"normal": scenario},
				"coverageReviews": map[string]any{"retry": review},
			}
			doc := map[string]any{"schemaVersion": "1", "usecases": map[string]any{"flow": usecase}}
			err := c.Validate("document", doc)
			if change == "valid" || change == "states" || change == "compositeStates" {
				if err != nil {
					t.Fatalf("valid review rejected: %v", err)
				}
			} else if err == nil {
				t.Fatalf("schema accepted invalid review %s", change)
			}
		})
	}
}
