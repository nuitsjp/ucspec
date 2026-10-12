package adapters

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/analysis"
	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/compiler"
	"github.com/nuitsjp/ucspec/internal/project"
	"github.com/nuitsjp/ucspec/internal/report"
)

func parse(t *testing.T, raw string) any {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// These integration tests require the REAL downloaded modules. They must run in
// Windows CI before release; they are not counted among offline core tests.
func TestYAMLDecoding(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	data, err := c.Decode([]byte("schemaVersion: '1'\nactors:\n  担当者: {}\n"), ".yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if err = c.Validate("document", doc); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"a: 1\na: 2", "a: &a 1\nb: *a", "a: 1\n---\nb: 2", "1: text"} {
		if _, err = c.Decode([]byte(bad), ".yaml"); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
}

const validDocument = `{
	"schemaVersion":"1",
	"start":{"to":"顧客管理"},
	"actors":{"担当者":{"description":"社内担当者"}},
	"events":{"終了":{"actors":["担当者"]}},
	"states":{"顧客管理":{"start":{"action":"一覧を開く","to":"一覧"},"layoutLayer":0,"states":{
		"一覧":{"screen":"顧客一覧","events":{"開く":{"actors":["担当者"],"defaultVisible":true,"decision":{"label":"有無","branches":{"あり":{"to":"詳細"},"なし":{"action":"知らせる"}}}}}},
		"詳細":{"events":{"戻る":{"actors":["担当者"],"to":"一覧"}}}}}},
	"usecasePackages":{"customer":{"name":"顧客"}},
	"usecases":{"view":{"name":"閲覧","goal":"確認する","actor":"担当者","package":"customer","starts":["一覧"],"ends":["一覧:開く"],"scope":["一覧:開く"],
		"scenarios":{"found":{"name":"見つかる","goal":"確認する","expectedOutcome":"表示される","start":"一覧","steps":[{"event":"開く","branches":["あり"]}]}}}},
	"regressions":{"back":{"name":"戻る","goal":"戻る","expectedOutcome":"一覧","actor":"担当者","start":"詳細","steps":[{"event":"戻る"},{"event":"開く","branches":["なし"]},{"event":"削除","reject":true}]}},
	"commonRules":{"error":{"name":"エラー","description":"共通メッセージを表示する"}}
}`
const validProject = `{"schemaVersion":"1","name":"Schema","files":["model.json"],"output":"artifacts","limits":{"steps":8,"paths":1000},"coverage":{"states":100,"compositeStates":100,"transitions":100,"pairs":100,"primePaths":100,"boundedPaths":100}}`

func TestSchemaAcceptsTheFormatAndRejectsRemovedFields(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Validate("document", parse(t, validDocument)); err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	if err := c.Validate("project", parse(t, validProject)); err != nil {
		t.Fatalf("valid project rejected: %v", err)
	}
	root := t.TempDir()
	for name, raw := range map[string]string{"project.json": validProject, "model.json": validDocument} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := compiler.Load(filepath.Join(root, "project.json"), c)
	if err != nil {
		t.Fatalf("document did not load through the production codec: %v", err)
	}
	if d := compiler.Validate(s); len(d) != 0 {
		t.Fatalf("valid document has problems: %+v", d)
	}
	for name, raw := range map[string]string{
		"variables":         `{"schemaVersion":"1","variables":{"x":{"domain":[true,false],"initial":true}}}`,
		"transitions":       `{"schemaVersion":"1","transitions":{"t":{"from":"a","to":"b","action":""}}}`,
		"choices":           `{"schemaVersion":"1","choices":{"c":{"label":"C"}}}`,
		"screens":           `{"schemaVersion":"1","screens":{"s":{"label":"S"}}}`,
		"properties":        `{"schemaVersion":"1","properties":{"p":{"kind":"reachable","description":"d","assert":true}}}`,
		"state label":       `{"schemaVersion":"1","states":{"A":{"label":"A"}}}`,
		"event source":      `{"schemaVersion":"1","states":{"A":{"events":{"e":{"actors":["x"],"source":"user"}}}}}`,
		"event when":        `{"schemaVersion":"1","states":{"A":{"events":{"e":{"actors":["x"],"when":true}}}}}`,
		"no actors":         `{"schemaVersion":"1","states":{"A":{"events":{"e":{"to":"A"}}}}}`,
		"to and decision":   `{"schemaVersion":"1","states":{"A":{"events":{"e":{"actors":["x"],"to":"A","decision":{"branches":{"a":{},"b":{}}}}}}}}`,
		"one branch":        `{"schemaVersion":"1","states":{"A":{"events":{"e":{"actors":["x"],"decision":{"branches":{"a":{}}}}}}}}`,
		"branch guard":      `{"schemaVersion":"1","states":{"A":{"events":{"e":{"actors":["x"],"decision":{"branches":{"a":{"guard":true},"b":{}}}}}}}}`,
		"bad state name":    `{"schemaVersion":"1","states":{"A/B":{}}}`,
		"bad event name":    `{"schemaVersion":"1","states":{"A":{"events":{"e>f":{"actors":["x"]}}}}}`,
		"negative layer":    `{"schemaVersion":"1","states":{"A":{"layoutLayer":-1}}}`,
		"step input":        `{"schemaVersion":"1","regressions":{"r":{"name":"n","goal":"g","expectedOutcome":"o","actor":"x","start":"A","steps":[{"event":"e","input":{}}]}}}`,
		"scenario expect":   `{"schemaVersion":"1","regressions":{"r":{"name":"n","goal":"g","expectedOutcome":"o","actor":"x","start":"A","steps":[{"event":"e"}],"expect":true}}}`,
		"blank goal":        `{"schemaVersion":"1","regressions":{"r":{"name":"n","goal":" ","expectedOutcome":"o","actor":"x","start":"A","steps":[{"event":"e"}]}}}`,
		"use-case end expr": `{"schemaVersion":"1","usecases":{"u":{"name":"n","goal":"g","actor":"x","starts":["A"],"end":true,"ends":["A:e"],"scope":["A:e"],"scenarios":{}}}}`,
	} {
		if err := c.Validate("document", parse(t, raw)); err == nil {
			t.Errorf("schema accepted %s", name)
		}
	}
	for name, raw := range map[string]string{
		"initial":         `{"initial":[{"state":"A"}]}`,
		"condition kind":  `{"coverage":{"conditions":100}}`,
		"limit too large": `{"limits":{"steps":257,"paths":1}}`,
		"old limits":      `{"limits":{"states":1,"steps":1,"paths":1,"combinations":1}}`,
	} {
		doc := parse(t, validProject).(map[string]any)
		for k, v := range parse(t, raw).(map[string]any) {
			doc[k] = v
		}
		if err := c.Validate("project", doc); err == nil {
			t.Errorf("project schema accepted %s", name)
		}
	}
}

func TestProductionCodecRejectsLookalikeNames(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	doc := `{"schemaVersion":"1","start":{"to":"Ａ"},"actors":{"人":{}},"states":{"A":{},"Ａ":{}}}`
	for name, raw := range map[string]string{"project.json": validProject, "model.json": doc} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
	}
	s, err := compiler.Load(filepath.Join(root, "project.json"), c)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range compiler.Validate(s) {
		if strings.Contains(d.Message, "looks the same") {
			return
		}
	}
	t.Fatal("full-width and half-width names were both accepted")
}

func TestSchemaCoverageReviews(t *testing.T) {
	c, err := Codec(assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []string{"valid", "states", "compositeStates", "kind", "removedKind", "status", "reason", "basisHash", "requirements", "duplicateRequirement", "scenario", "architectureRequirements"} {
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
			case "removedKind":
				review["kind"] = "conditions"
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
			scenario := map[string]any{"name": "normal", "goal": "complete", "expectedOutcome": "completed", "start": "A", "steps": []any{map[string]any{"event": "e"}}}
			usecase := map[string]any{
				"name": "flow", "goal": "complete", "actor": "user", "starts": []any{"A"}, "ends": []any{"A:e"},
				"scope": []any{"A:e"}, "scenarios": map[string]any{"normal": scenario}, "coverageReviews": map[string]any{"retry": review},
			}
			err := c.Validate("document", map[string]any{"schemaVersion": "1", "usecases": map[string]any{"flow": usecase}})
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
		t.Fatalf("analysis status %s: %+v", r.Status, r.Diagnostics)
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
