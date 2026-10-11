package adapters

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
)

func TestActualELKStartsAboveCyclesDisconnectedAndNestedNodes(t *testing.T) {
	s := &model.Spec{
		Project: model.Project{Initial: []model.Snapshot{{State: "a"}, {State: "b"}}},
		Document: model.Document{
			Screens: map[string]model.Screen{"main": {Label: "Main"}},
			States: map[string]model.State{
				"a":        {Label: "A", Screen: "main"},
				"b":        {Label: "B", Screen: "main", Initial: "first"},
				"first":    {Label: "First", Screen: "main", Parent: "b"},
				"second":   {Label: "Second", Screen: "main", Parent: "b"},
				"isolated": {Label: "Isolated", Screen: "main"},
			},
			Events: map[string]model.Event{"go": {Label: "Go", Source: "user"}},
			Transitions: map[string]model.Transition{
				"ab":   {From: "a", To: "b", Event: "go"},
				"ba":   {From: "second", To: "a", Event: "go"},
				"next": {From: "first", To: "second", Event: "go"},
				"back": {From: "second", To: "first", Event: "go"},
			},
			UseCases: map[string]model.UseCase{"nested": {Starts: []model.Snapshot{{State: "second"}}, Scope: []string{"ba", "ab"}}},
		},
	}
	var graphs []report.Graph
	layout := func(b []byte) ([]byte, error) {
		out, err := LayoutJSON(b)
		if err != nil {
			return nil, err
		}
		var g report.Graph
		if err := json.Unmarshal(out, &g); err != nil {
			return nil, err
		}
		graphs = append(graphs, g)
		return out, nil
	}
	page, err := report.Build(s, &model.Report{}, layout, assets.Files)
	if err != nil {
		t.Fatal(err)
	}
	assertActualCenteredStarts(t, page)
	for _, g := range graphs {
		assertInitialRow(t, g.Children)
	}
}

func assertInitialRow(t *testing.T, children []report.Shape) {
	t.Helper()
	for _, marker := range children {
		if !strings.HasPrefix(marker.ID, "__start_") && !strings.HasPrefix(marker.ID, "__initial_") {
			continue
		}
		for _, node := range children {
			if strings.HasPrefix(node.ID, "__start_") || strings.HasPrefix(node.ID, "__initial_") {
				continue
			}
			if marker.Y+marker.Height > node.Y+.01 {
				t.Fatalf("start %s at %.2f..%.2f is not above %s at %.2f", marker.ID, marker.Y, marker.Y+marker.Height, node.ID, node.Y)
			}
		}
	}
}

func TestActualELKDownwardChain(t *testing.T) {
	s := &model.Spec{Project: model.Project{Initial: []model.Snapshot{{State: "a"}}}, Document: model.Document{
		Screens:     map[string]model.Screen{"main": {Label: "Main"}},
		States:      map[string]model.State{"a": {Label: "A", Screen: "main"}, "b": {Label: "B", Screen: "main"}},
		Events:      map[string]model.Event{"go": {Label: "Go", Source: "user"}},
		Transitions: map[string]model.Transition{"go": {From: "a", To: "b", Event: "go", DefaultVisible: true}},
	}}
	layout := func(b []byte) ([]byte, error) {
		out, err := LayoutJSON(b)
		if err != nil {
			return nil, err
		}
		var g report.Graph
		if err := json.Unmarshal(out, &g); err != nil {
			return nil, err
		}
		a, bnode := choiceShape(g.Children, "a"), choiceShape(g.Children, "b")
		if bnode.Y < a.Y+a.Height {
			t.Fatal("flow was not laid out downward")
		}
		return out, nil
	}
	if _, err := report.Build(s, &model.Report{}, layout, assets.Files); err != nil {
		t.Fatal(err)
	}
}

func TestActualELKEventEntersChoiceDownwardDespiteGuardedReturn(t *testing.T) {
	s := &model.Spec{Project: model.Project{Initial: []model.Snapshot{{State: "search"}}}, Document: model.Document{
		Screens: map[string]model.Screen{"main": {Label: "Main"}},
		States: map[string]model.State{
			"search":  {Label: "Search", Screen: "main"},
			"view":    {Label: "View", Screen: "main", Initial: "viewing"},
			"viewing": {Label: "Viewing", Screen: "main", Parent: "view"},
		},
		Choices: map[string]model.Choice{"availability": {Label: "Availability"}},
		Events:  map[string]model.Event{"open": {Label: "Open", Source: "user"}},
		Transitions: map[string]model.Transition{
			"open": {From: "search", To: "availability", Event: "open", DefaultVisible: true},
			"view": {From: "availability", To: "view", GuardLabel: "Exists", Action: "Show customer", DefaultVisible: true},
			"gone": {From: "availability", To: "search", GuardLabel: "Missing", Action: "Search and alert", DefaultVisible: true},
		},
	}}
	layout := func(b []byte) ([]byte, error) {
		out, err := LayoutJSON(b)
		if err != nil {
			return nil, err
		}
		var g report.Graph
		if err := json.Unmarshal(out, &g); err != nil {
			return nil, err
		}
		search, choice, view := choiceShape(g.Children, "search"), choiceShape(g.Children, "availability"), choiceShape(g.Children, "view")
		if choice.Y < search.Y+search.Height || view.Y < choice.Y+choice.Height {
			t.Fatal("guarded return made the event flow upward")
		}
		assertInitialRow(t, g.Children)
		return out, nil
	}
	if _, err := report.Build(s, &model.Report{}, layout, assets.Files); err != nil {
		t.Fatal(err)
	}
}
