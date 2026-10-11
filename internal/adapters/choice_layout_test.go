package adapters

import (
	"encoding/json"
	"testing"

	"github.com/nuitsjp/ucspec/internal/assets"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/report"
	"github.com/nuitsjp/ucspec/internal/testkit/choicefixture"
)

func choiceShape(nodes []report.Shape, id string) *report.Shape {
	for i := range nodes {
		if nodes[i].ID == id {
			return &nodes[i]
		}
		if found := choiceShape(nodes[i].Children, id); found != nil {
			return found
		}
	}
	return nil
}

func TestActualELKChoiceNodeBounds(t *testing.T) {
	s := choicefixture.ChoiceSpec()
	var laidOut []report.Graph
	layout := func(b []byte) ([]byte, error) {
		out, err := LayoutJSON(b)
		if err != nil {
			return nil, err
		}
		var g report.Graph
		if err := json.Unmarshal(out, &g); err != nil {
			return nil, err
		}
		laidOut = append(laidOut, g)
		return out, nil
	}
	if _, err := report.Build(s, &model.Report{}, layout, assets.Files); err != nil {
		t.Fatal(err)
	}
	if len(laidOut) == 0 {
		t.Fatal("system graph was not laid out")
	}
	choice := choiceShape(laidOut[0].Children, "result")
	if choice == nil {
		t.Fatal("ELK graph has no choice node")
	}
	if choice.Width <= 0 || choice.Height <= 0 || choice.X < 0 || choice.Y < 0 || choice.X+choice.Width > laidOut[0].Width || choice.Y+choice.Height > laidOut[0].Height {
		t.Fatalf("choice node is outside the ELK graph: %#v in %.2fx%.2f", choice, laidOut[0].Width, laidOut[0].Height)
	}
}
