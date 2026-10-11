package report_test

import (
	"bytes"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

func visibilitySpec() (*model.Spec, *model.Report) {
	s := &model.Spec{
		Project: model.Project{
			Name:    "Visibility variants",
			Initial: []model.Snapshot{{State: "start"}},
		},
		Document: model.Document{
			Screens: map[string]model.Screen{"main": {Label: "Main"}},
			States: map[string]model.State{
				"start":  {Label: "Start", Screen: "main", Events: []string{"go"}},
				"box":    {Label: "Box", Screen: "main", Initial: "inside"},
				"inside": {Label: "Inside", Screen: "main", Parent: "box", Events: []string{"go"}},
				"middle": {Label: "Middle", Screen: "main", Events: []string{"go", "ping"}},
				"done":   {Label: "Done", Screen: "main"},
			},
			Choices: map[string]model.Choice{
				"branch": {Label: "Branch", Parent: "box"},
			},
			Events: map[string]model.Event{
				"go":   {Label: "Go", Source: "user"},
				"ping": {Label: "Ping", Source: "user"},
			},
			Transitions: map[string]model.Transition{
				"enter": {
					From: "start", To: "box", Event: "go", Action: "Enter", DefaultVisible: true,
				},
				"choose": {
					From: "inside", To: "branch", Event: "go", Action: "Choose", DefaultVisible: true,
				},
				"hiddenOne": {
					From: "branch", To: "middle", Action: "First hidden path",
				},
				"hiddenTwo": {
					From: "middle", To: "done", Action: "Second hidden path",
				},
				"hiddenInternal": {
					From: "middle", To: "middle", Kind: "internal", Event: "ping", Action: "Hidden row",
				},
			},
			UseCases: map[string]model.UseCase{
				"flow": {
					Name:   "Flow",
					Starts: []model.Snapshot{{State: "start"}},
					Scope:  []string{"enter", "choose", "hiddenOne", "hiddenTwo", "hiddenInternal"},
				},
			},
		},
	}
	r := &model.Report{
		Name: "Visibility variants",
		Scenarios: []model.ScenarioResult{
			{
				ID: "flow/known", UseCase: "flow", Valid: true,
				Witness: model.Witness{Start: model.Snapshot{State: "start"}, Steps: []model.Step{{Transition: "hiddenTwo"}}},
			},
		},
		UseCases: []model.UseCaseResult{
			{
				ID: "flow",
				Coverage: []model.Coverage{{Kind: "transitions", Missing: []model.Requirement{
					{ID: "missing-hidden", Witness: &model.Witness{Steps: []model.Step{{Transition: "hiddenInternal"}}}},
				}}},
			},
		},
		Checks: []model.Check{{
			ID: "counterexample", Status: "failed",
			Witness: &model.Witness{Steps: []model.Step{{Transition: "hiddenOne"}, {Transition: "hiddenInternal"}}},
		}},
		Diagnostics: []model.Diagnostic{{
			Code: "counterexample", Severity: "warning",
			Witness: &model.Witness{Steps: []model.Step{{Transition: "hiddenOne"}, {Transition: "hiddenInternal"}}},
		}},
	}
	return s, r
}

type visibilityLayoutCall struct {
	Graph report.Graph
}

func parseVisibilityTemplates(t *testing.T, page []byte) map[string]string {
	t.Helper()
	matches := regexp.MustCompile(`(?s)<template id="([^"]+)">(.*?)</template>`).FindAllSubmatch(page, -1)
	if len(matches) == 0 {
		t.Fatal("no diagram templates")
	}
	templates := map[string]string{}
	for _, match := range matches {
		templates[string(match[1])] = string(match[2])
	}
	return templates
}

func parseDiagramVariants(t *testing.T, page []byte) map[string]map[string]string {
	t.Helper()
	const prefix = `<script id="spec-data" type="application/json">`
	start := bytes.Index(page, []byte(prefix))
	if start < 0 {
		t.Fatal("spec-data is missing")
	}
	start += len(prefix)
	rest := page[start:]
	end := bytes.Index(rest, []byte(`</script>`))
	if end < 0 {
		t.Fatal("spec-data is not closed")
	}
	var data struct {
		DiagramVariants map[string]map[string]string `json:"diagramVariants"`
	}
	if err := json.Unmarshal(rest[:end], &data); err != nil {
		t.Fatalf("invalid spec-data: %v", err)
	}
	return data.DiagramVariants
}

func visibilityTransitionIDs(svg string) map[string]bool {
	ids := map[string]bool{}
	for _, match := range regexp.MustCompile(`data-transition="([^"]+)"`).FindAllStringSubmatch(svg, -1) {
		ids[match[1]] = true
	}
	return ids
}

func visibilityShape(nodes []report.Shape, id string) *report.Shape {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
		if found := visibilityShape(nodes[i].Children, id); found != nil {
			return found
		}
	}
	return nil
}

func visibilityEdgeIDs(g report.Graph) []string {
	ids := []string{}
	for _, edge := range g.Edges {
		ids = append(ids, edge.ID)
	}
	sort.Strings(ids)
	return ids
}

func TestVisibilityBuildsFilteredAndPrecomputedVariants(t *testing.T) {
	s, r := visibilitySpec()
	before := model.JSON(s) + model.JSON(r)
	var calls []visibilityLayoutCall
	layout := func(input []byte) ([]byte, error) {
		var graph report.Graph
		if err := json.Unmarshal(input, &graph); err != nil {
			return nil, err
		}
		calls = append(calls, visibilityLayoutCall{Graph: graph})
		return testkit.FixtureLayout(input)
	}
	page, err := report.Build(s, r, layout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	if before != model.JSON(s)+model.JSON(r) {
		t.Fatal("layout variants mutated the model or report")
	}

	variants := parseDiagramVariants(t, page)
	system, ok := variants[""]
	if !ok {
		t.Fatalf("system diagram variants are missing: %#v", variants)
	}
	flow, ok := variants["flow"]
	if !ok {
		t.Fatalf("use-case diagram variants are missing: %#v", variants)
	}
	for name, index := range map[string]map[string]string{"system": system, "flow": flow} {
		if index["[]"] != map[string]string{"system": "system", "flow": "uc-flow"}[name] {
			t.Fatalf("%s normal variant: %#v", name, index)
		}
		wantAll := map[string]string{"system": "system-all", "flow": "uc-flow-all"}[name]
		if index["*"] != wantAll {
			t.Fatalf("%s all variant: got %q want %q", name, index["*"], wantAll)
		}
	}
	for _, key := range []string{`["hiddenInternal"]`, `["hiddenOne"]`, `["hiddenTwo"]`} {
		id, ok := flow[key]
		if !ok {
			t.Fatalf("selected flow variant %s is missing: %#v", key, flow)
		}
		if !strings.HasPrefix(id, "uc-flow-selected-") {
			t.Fatalf("selected flow variant has wrong ID %q", id)
		}
	}
	for _, key := range []string{`["hiddenInternal"]`, `["hiddenOne"]`, `["hiddenTwo"]`, `["hiddenInternal","hiddenOne"]`} {
		if system[key] == "" {
			t.Fatalf("system selected variant %s is missing: %#v", key, system)
		}
	}

	templates := parseVisibilityTemplates(t, page)
	for _, id := range []string{"system", "system-all", "uc-flow", "uc-flow-all"} {
		if _, ok := templates["diagram-"+id]; !ok {
			t.Fatalf("template diagram-%s is missing", id)
		}
	}
	normal := templates["diagram-system"]
	all := templates["diagram-system-all"]
	if ids := visibilityTransitionIDs(normal); !ids["enter"] || !ids["choose"] || ids["hiddenOne"] || ids["hiddenTwo"] || ids["hiddenInternal"] {
		t.Fatalf("normal diagram transition set: %#v", ids)
	}
	allIDs := visibilityTransitionIDs(all)
	for _, id := range []string{"enter", "choose", "hiddenOne", "hiddenTwo", "hiddenInternal"} {
		if !allIDs[id] {
			t.Fatalf("all diagram omitted transition %s: %#v", id, allIDs)
		}
	}
	if !strings.Contains(all, `data-state="start"`) || !strings.Contains(all, `data-state="box"`) || !strings.Contains(all, `data-state="inside"`) || !strings.Contains(all, `data-state="middle"`) || !strings.Contains(all, `data-state="done"`) || !strings.Contains(all, `data-choice="branch"`) {
		t.Fatal("all diagram did not retain scoped states and choice")
	}
	selected := templates["diagram-"+flow[`["hiddenTwo"]`]]
	selectedIDs := visibilityTransitionIDs(selected)
	if !selectedIDs["hiddenTwo"] || selectedIDs["hiddenOne"] || selectedIDs["hiddenInternal"] {
		t.Fatalf("selected diagram transition set: %#v", selectedIDs)
	}

	if len(calls) < 6 {
		t.Fatalf("got %d ELK layouts, want normal/all/selected for both views", len(calls))
	}
	normalEdges := visibilityEdgeIDs(calls[0].Graph)
	if !containsVisibilityID(normalEdges, "enter") || !containsVisibilityID(normalEdges, "choose") || containsVisibilityID(normalEdges, "hiddenOne") || containsVisibilityID(normalEdges, "hiddenTwo") {
		t.Fatalf("normal ELK input edge set: %#v", normalEdges)
	}
	allEdges := visibilityEdgeIDs(calls[1].Graph)
	if !containsVisibilityID(allEdges, "hiddenOne") || !containsVisibilityID(allEdges, "hiddenTwo") {
		t.Fatalf("all-transition ELK input omitted hidden external edges: %#v", allEdges)
	}
	for _, id := range []string{"start", "box", "inside", "middle", "done", "branch"} {
		if visibilityShape(calls[0].Graph.Children, id) == nil {
			t.Fatalf("normal ELK input omitted scoped vertex %s", id)
		}
	}
	for i, call := range calls {
		if got := call.Graph.Options["elk.layered.nodePlacement.strategy"]; got != "NETWORK_SIMPLEX" {
			t.Fatalf("layout %d root placement strategy: %#v", i, got)
		}
		box := visibilityShape(call.Graph.Children, "box")
		if box == nil {
			t.Fatalf("layout %d has no composite box", i)
		}
		if got := box.Options["elk.layered.nodePlacement.strategy"]; got != "NETWORK_SIMPLEX" {
			t.Fatalf("layout %d composite placement strategy: %#v", i, got)
		}
	}
	normalHeight, allHeight := 0.0, 0.0
	for _, call := range calls {
		if middle := visibilityShape(call.Graph.Children, "middle"); middle != nil {
			if middle.Height > allHeight {
				allHeight = middle.Height
			}
			if normalHeight == 0 || middle.Height < normalHeight {
				normalHeight = middle.Height
			}
		}
	}
	if normalHeight <= 0 || allHeight <= normalHeight {
		t.Fatalf("hidden internal row did not change the state height: normal=%.2f all=%.2f", normalHeight, allHeight)
	}
}

func containsVisibilityID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
