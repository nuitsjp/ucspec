package adapters

import (
	"encoding/json"
	"strings"
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

func TestActualELKSampleLayoutLayersIncludingScopedDiagrams(t *testing.T) {
	s := testkit.Sample(t)
	for id, want := range map[string]int{"顧客管理/顧客検索": 0, "顧客管理/顧客閲覧": 1, "顧客管理/顧客編集": 2} {
		layer := s.States[id].LayoutLayer
		if layer == nil || *layer != want {
			t.Fatalf("sample state %s does not specify layer %d", id, want)
		}
	}
	checkActualLayerLayout(t, s)
}

func TestActualELKLayersWithinCompositeState(t *testing.T) {
	s := testkit.Compile(t, `{"schemaVersion":"1","start":{"to":"作業"},"actors":{"人":{}},"states":{"作業":{"start":{"to":"準備"},"states":{
	 "準備":{"layoutLayer":0,"events":{"進む":{"actors":["人"],"defaultVisible":true,"to":"次"}}},
	 "別":{"layoutLayer":0},
	 "次":{"layoutLayer":2,"events":{"進む":{"actors":["人"],"defaultVisible":true,"to":"最後"}}},
	 "最後":{"layoutLayer":5,"events":{"戻る":{"actors":["人"],"defaultVisible":true,"to":"作業"}}}}}}}`)
	checkActualLayerLayout(t, s)
}
