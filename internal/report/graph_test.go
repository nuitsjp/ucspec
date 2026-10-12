package report_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/analysis"
	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

func templateSVG(t *testing.T, page []byte, id string) string {
	t.Helper()
	text := string(page)
	open := `<template id="` + id + `">`
	start := strings.Index(text, open)
	if start < 0 {
		t.Fatalf("template %s missing", id)
	}
	start += len(open)
	end := strings.Index(text[start:], "</template>")
	if end < 0 {
		t.Fatalf("template %s is not closed", id)
	}
	return text[start : start+end]
}

// A composite with a start, a staying event, a visible decision, a hidden
// event entering its own decision and a state whose screen differs from its name.
const drawing = `{"schemaVersion":"1","start":{"to":"業務"},"actors":{"人":{}},
 "states":{
  "業務":{"start":{"action":"一覧を開く","to":"一覧"},"states":{
   "一覧":{"screen":"<一覧画面>","events":{
     "更新":{"actors":["人"],"defaultVisible":true,"action":"再表示する"},
     "開く":{"actors":["人"],"defaultVisible":true,"decision":{"label":"<有無>","branches":{"あり":{"to":"詳細"},"なし":{}}}},
     "離れる":{"actors":["人"],"decision":{"label":"確認","branches":{"はい":{"to":"外"},"いいえ":{}}}}}},
   "詳細":{"events":{"戻る":{"actors":["人"],"defaultVisible":true,"to":"一覧"}}}}},
  "外":{"events":{"入る":{"actors":["人"],"to":"業務"}}}},
 "usecases":{"u":{"name":"U","goal":"G","actor":"人","starts":["一覧"],"ends":["一覧:開く"],"scope":["一覧:開く"],
  "scenarios":{"s":{"name":"s","goal":"g","expectedOutcome":"o","start":"一覧","steps":[{"event":"開く","branches":["あり"]}]}}}}}`

func buildDrawing(t *testing.T) (*model.Spec, []byte) {
	t.Helper()
	s := testkit.Compile(t, drawing)
	r := analysis.Analyze(s, "test")
	b, err := report.Build(s, r, testkit.FixtureLayout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	return s, b
}

func TestDiagramDrawsStatesDecisionsAndInternalEvents(t *testing.T) {
	_, b := buildDrawing(t)
	svg := templateSVG(t, b, "diagram-system")
	for _, want := range []string{
		`data-state="業務"`, `data-state="業務/一覧"`, `class="node composite"`,
		`data-decision="業務/一覧:開く?"`, `&lt;有無&gt;`,
		`data-transition="業務/一覧:開く&gt;あり"`, `class="edge decision-edge"`,
		`data-kind="internal"`, `再表示する`,
		`class="initial-edge" data-transition="業務:[開始]"`, `/ 一覧を開く`,
		`画面: &lt;一覧画面&gt;`,
		`class="start-node" data-start-state="業務"`,
	} {
		if !strings.Contains(svg, want) {
			t.Errorf("system diagram lacks %s", want)
		}
	}
	if strings.Contains(svg, `data-decision="業務/一覧:離れる?"`) {
		t.Error("a decision of a hidden event must not be laid out in the normal diagram")
	}
	if strings.Contains(svg, "<有無>") || strings.Contains(svg, "<一覧画面>") {
		t.Error("labels are not escaped")
	}
	all := templateSVG(t, b, "diagram-system-all")
	if !strings.Contains(all, `data-decision="業務/一覧:離れる?"`) || !strings.Contains(all, `data-transition="業務/一覧:離れる&gt;いいえ"`) {
		t.Error("the all-transitions diagram must show hidden events and their decisions")
	}
}

func TestUseCaseDiagramShowsScopeAndStarts(t *testing.T) {
	_, b := buildDrawing(t)
	svg := templateSVG(t, b, "diagram-uc-u")
	if !strings.Contains(svg, `class="start-node" data-start-state="業務/一覧"`) || !strings.Contains(svg, `data-transition="業務/一覧:開く"`) {
		t.Error("use-case diagram lacks its start or scoped event")
	}
	for _, outside := range []string{`data-transition="業務/一覧:更新"`, `data-transition="外:入る"`} {
		if strings.Contains(svg, outside) {
			t.Errorf("use-case diagram shows %s outside its scope", outside)
		}
	}
}

func TestDiagramVariantsCoverHiddenSelections(t *testing.T) {
	_, b := buildDrawing(t)
	_, data, _ := strings.Cut(string(b), `<script id="spec-data" type="application/json">`)
	data, _, _ = strings.Cut(data, `</script>`)
	var payload struct {
		Variants map[string]map[string]string `json:"diagramVariants"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatal(err)
	}
	system := payload.Variants[""]
	if system["[]"] != "system" || system["*"] != "system-all" {
		t.Fatalf("base variants: %v", system)
	}
	hidden, _ := json.Marshal([]string{"業務/一覧:離れる"})
	if id, ok := system[string(hidden)]; !ok || !strings.Contains(string(b), `<template id="diagram-`+id+`">`) {
		t.Fatalf("selecting a hidden event has no precomputed diagram: %v", system)
	}
}

func TestNestedEdgeContainerOffsetsAreAppliedToRoutes(t *testing.T) {
	s := testkit.Compile(t, drawing)
	r := analysis.Analyze(s, "test")
	layout := func(b []byte) ([]byte, error) {
		var g report.Graph
		if err := json.Unmarshal(b, &g); err != nil {
			return nil, err
		}
		g.Width, g.Height = 800, 600
		var locate func([]report.Shape)
		locate = func(nodes []report.Shape) {
			for i := range nodes {
				nodes[i].X, nodes[i].Y = 30, 40
				if nodes[i].ID == "業務" {
					nodes[i].X, nodes[i].Y = 100, 50
					nodes[i].Width, nodes[i].Height = 600, 500
				}
				locate(nodes[i].Children)
				for j := range nodes[i].Edges {
					nodes[i].Edges[j].Sections = []report.Section{{Start: report.Point{X: 10, Y: 20}, End: report.Point{X: 60, Y: 70}}}
				}
			}
		}
		locate(g.Children)
		for i := range g.Edges {
			g.Edges[i].Sections = []report.Section{{Start: report.Point{X: 10, Y: 20}, End: report.Point{X: 60, Y: 70}}}
			if g.Edges[i].ID == "業務/詳細:戻る" {
				g.Edges[i].Container = "業務"
			}
		}
		return json.Marshal(g)
	}
	b, err := report.Build(s, r, layout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	svg := templateSVG(t, b, "diagram-system")
	start := strings.Index(svg, `data-transition="業務/詳細:戻る"`)
	if start < 0 {
		t.Fatal("nested edge missing")
	}
	edge := svg[start : start+strings.Index(svg[start:], `</g>`)]
	if !strings.Contains(edge, `d="M110.00 70.00 L160.00 120.00 `) {
		t.Fatalf("container offset not applied: %s", edge)
	}
	if !strings.Contains(svg, `<g class="initial-edge" data-transition="業務:[開始]" data-parent-state="業務" data-initial-state="業務/一覧"><title>開始: 一覧</title><path marker-end="url(#arrow)" d="M110.00 70.00 L160.00 120.00 `) {
		t.Fatal("start edge lost its owning composite offset")
	}
}
