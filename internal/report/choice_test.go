package report_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit"
	"github.com/nuitsjp/ucspec/internal/testkit/choicefixture"
)

func findShape(nodes []report.Shape, id string) *report.Shape {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
		if found := findShape(nodes[i].Children, id); found != nil {
			return found
		}
	}
	return nil
}

func TestChoiceScopeParentsDiamondAndLabels(t *testing.T) {
	s := choicefixture.ChoiceSpec()
	s.States["workspace"] = model.State{Label: "Workspace", Screen: "screen", Initial: "ready"}
	s.States["ready"] = model.State{Label: "Ready", Screen: "screen", Parent: "workspace", Events: []string{"search"}}
	s.States["outside"] = model.State{Label: "Outside", Screen: "screen"}
	s.Choices["result"] = model.Choice{Label: "<choice>", Parent: "workspace"}

	var graphs []report.Graph
	layout := func(b []byte) ([]byte, error) {
		var g report.Graph
		if err := json.Unmarshal(b, &g); err != nil {
			return nil, err
		}
		graphs = append(graphs, g)
		return testkit.FixtureLayout(b)
	}
	r := &model.Report{Name: s.Project.Name, Status: "verified"}
	b, err := report.Build(s, r, layout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	var allGraph *report.Graph
	for i := range graphs {
		if len(graphs[i].Edges) == 4 && findShape(graphs[i].Children, "result") != nil && findShape(graphs[i].Children, "outside") == nil {
			allGraph = &graphs[i]
			break
		}
	}
	if allGraph == nil {
		t.Fatalf("all-transition graph was not laid out: %d graphs", len(graphs))
	}
	choice := findShape(allGraph.Children, "result")
	if choice == nil {
		t.Fatal("scoped choice is missing")
	}
	if choice.Width <= 0 || choice.Height <= 0 {
		t.Fatalf("choice dimensions missing: %#v", choice)
	}
	if findShape(allGraph.Children, "workspace") == nil {
		t.Fatal("choice parent was not included in scope")
	}
	if findShape(allGraph.Children, "outside") != nil {
		t.Fatal("out-of-scope state was included")
	}
	if len(allGraph.Edges) != 4 {
		t.Fatalf("scoped transitions changed: %d", len(allGraph.Edges))
	}
	labels := map[string]string{}
	for _, edge := range allGraph.Edges {
		if len(edge.Labels) > 0 {
			labels[edge.ID] = edge.Labels[0].Text
		}
	}
	if labels["search"] != "Search button\n/ Fetch" {
		t.Fatalf("event/action label missing: %#v", labels["search"])
	}
	if !strings.Contains(labels["found"], "[Found]") || !strings.Contains(labels["found"], "/ Display") || strings.Contains(labels["found"], "ユーザー") {
		t.Fatalf("choice branch label is not eventless: %#v", labels["found"])
	}
	if !strings.Contains(labels["empty"], "[Empty]") || !strings.Contains(labels["empty"], "/ Alert") {
		t.Fatalf("second choice branch label missing: %#v", labels["empty"])
	}
	svg := templateSVG(t, b, "diagram-uc-search-all")
	if !strings.Contains(svg, `class="choice" data-choice="result"`) || !strings.Contains(svg, "<polygon") {
		t.Fatal("choice was not rendered as a diamond")
	}
	if strings.Contains(svg, "<choice>") || !strings.Contains(svg, "&lt;choice&gt;") {
		t.Fatal("choice label was not HTML escaped")
	}
}
