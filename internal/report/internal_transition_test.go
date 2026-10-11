package report_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

func TestInternalTransitionsStayInsideStateAndRespectScope(t *testing.T) {
	s := &model.Spec{
		Project: model.Project{Initial: []model.Snapshot{{State: "search"}}},
		Document: model.Document{
			Screens: map[string]model.Screen{"main": {Label: "Main"}},
			States:  map[string]model.State{"search": {Label: "Search", Screen: "main"}},
			Events:  map[string]model.Event{"search": {Label: "Search", Source: "user"}},
			Transitions: map[string]model.Transition{
				"found": {From: "search", To: "search", Event: "search", Kind: "internal", DefaultVisible: true, GuardLabel: "Found", Action: "Show results"},
				"empty": {From: "search", To: "search", Event: "search", Kind: "internal", GuardLabel: "Empty", Action: `<script>alert(1)</script>`},
			},
			UseCases: map[string]model.UseCase{"find": {Starts: []model.Snapshot{{State: "search"}}, Scope: []string{"found", "empty"}}},
		},
	}
	before := model.JSON(s)
	var requests []report.Graph
	layout := func(b []byte) ([]byte, error) {
		var g report.Graph
		if err := json.Unmarshal(b, &g); err != nil {
			return nil, err
		}
		requests = append(requests, g)
		return testkit.FixtureLayout(b)
	}
	page, err := report.Build(s, &model.Report{}, layout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	if before != model.JSON(s) {
		t.Fatal("internal transition rendering mutated the model")
	}
	for _, g := range requests {
		for _, edge := range g.Edges {
			if edge.ID == "found" || edge.ID == "empty" {
				t.Fatal("internal transition became a routed self-loop")
			}
		}
	}
	normal := templateSVG(t, page, "diagram-system")
	if transitionCounts(normal)["found"] != 1 || transitionCounts(normal)["empty"] != 0 {
		t.Fatal("normal diagram did not filter hidden internal transitions")
	}
	all := templateSVG(t, page, "diagram-system-all")
	if transitionCounts(all)["found"] != 1 || transitionCounts(all)["empty"] != 1 {
		t.Fatal("all-transition diagram omitted an internal transition")
	}
	if !strings.Contains(all, `class="edge internal-transition transition-hidden" data-transition="empty" data-default-visible="false" aria-hidden="true" data-kind="internal"`) {
		t.Fatal("all-transition internal visibility metadata missing")
	}
	if strings.Contains(all, s.Transitions["empty"].Action) || !strings.Contains(all, "&lt;script&gt;") {
		t.Fatal("internal transition text was not escaped")
	}
	scoped := templateSVG(t, page, "diagram-uc-find")
	if transitionCounts(scoped)["found"] != 1 || transitionCounts(scoped)["empty"] != 0 {
		t.Fatal("normal use-case diagram did not filter hidden internal transitions")
	}
	scopedAll := templateSVG(t, page, "diagram-uc-find-all")
	if transitionCounts(scopedAll)["found"] != 1 || transitionCounts(scopedAll)["empty"] != 1 {
		t.Fatal("all-transition use-case diagram escaped its scope")
	}
	normalHeight := findShape(requests[0].Children, "search").Height
	allHeight := findShape(requests[1].Children, "search").Height
	if allHeight <= normalHeight {
		t.Fatalf("hidden internal row has no reserved-space difference: normal=%.2f all=%.2f", normalHeight, allHeight)
	}
}
