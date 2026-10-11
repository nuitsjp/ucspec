package adapters

import (
	"encoding/json"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

func assertLayoutLayerOrder(t *testing.T, s *model.Spec, children []report.Shape) {
	t.Helper()
	for _, upper := range children {
		u := s.States[upper.ID].LayoutLayer
		if u == nil {
			continue
		}
		for _, lower := range children {
			l := s.States[lower.ID].LayoutLayer
			if l != nil && *u < *l && upper.Y+upper.Height > lower.Y+.01 {
				t.Fatalf("layer %d state %s ends at %.2f; layer %d state %s starts at %.2f", *u, upper.ID, upper.Y+upper.Height, *l, lower.ID, lower.Y)
			}
		}
	}
	for _, node := range children {
		assertLayoutLayerOrder(t, s, node.Children)
	}
}

func checkActualLayerLayout(t *testing.T, s *model.Spec) {
	t.Helper()
	layout := func(b []byte) ([]byte, error) {
		out, err := LayoutJSON(b)
		if err != nil {
			return nil, err
		}
		var g report.Graph
		if err := json.Unmarshal(out, &g); err != nil {
			return nil, err
		}
		assertInitialRow(t, g.Children)
		assertLayoutLayerOrder(t, s, g.Children)
		return out, nil
	}
	if _, err := report.Build(s, &model.Report{}, layout, assets.Files); err != nil {
		t.Fatal(err)
	}
}

func TestActualELKSampleLayoutLayersIncludingScopedDiagrams(t *testing.T) {
	s := testkit.Sample(t)
	for id, want := range map[string]int{"search": 0, "notificationSettings": 0, "view": 1, "edit": 2} {
		layer := s.States[id].LayoutLayer
		if layer == nil || *layer != want {
			t.Fatalf("sample state %s does not specify layer %d", id, want)
		}
	}
	checkActualLayerLayout(t, s)
}

func TestActualELKLayersWithinCompositeState(t *testing.T) {
	zero, second, fifth := 0, 2, 5
	s := &model.Spec{Project: model.Project{Initial: []model.Snapshot{{State: "workspace"}}}, Document: model.Document{
		Screens: map[string]model.Screen{"main": {Label: "Main"}},
		States: map[string]model.State{
			"workspace": {Label: "Workspace", Screen: "main", Initial: "ready"},
			"ready":     {Label: "Ready", Screen: "main", Parent: "workspace", LayoutLayer: &zero},
			"alternate": {Label: "Alternate", Screen: "main", Parent: "workspace", LayoutLayer: &zero},
			"next":      {Label: "Next", Screen: "main", Parent: "workspace", LayoutLayer: &second},
			"last":      {Label: "Last", Screen: "main", Parent: "workspace", LayoutLayer: &fifth},
		},
		Events: map[string]model.Event{"go": {Label: "Go", Source: "user"}},
		Transitions: map[string]model.Transition{
			"forward": {From: "ready", To: "next", Event: "go"},
			"finish":  {From: "next", To: "last", Event: "go"},
			"back":    {From: "last", To: "workspace", Event: "go"},
		},
	}}
	checkActualLayerLayout(t, s)
}
