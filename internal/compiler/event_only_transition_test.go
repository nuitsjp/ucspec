package compiler

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
)

func TestEventOnlyTransitionLoadsAndApplies(t *testing.T) {
	root := t.TempDir()
	project := model.Project{
		SchemaVersion: "1",
		Name:          "event only",
		Files:         []string{"model.json"},
		Output:        "artifacts",
		Initial:       []model.Snapshot{{State: "start"}},
	}
	doc := model.Document{
		SchemaVersion: "1",
		Screens: map[string]model.Screen{
			"main": {Label: "Main"},
		},
		States: map[string]model.State{
			"start": {Label: "Start", Screen: "main", Events: []string{"advance"}},
			"done":  {Label: "Done", Screen: "main", Terminal: true},
		},
		Events: map[string]model.Event{
			"advance": {Label: "Advance", Source: "user"},
		},
		Transitions: map[string]model.Transition{
			"advance": {From: "start", To: "done", Event: "advance", Action: ""},
		},
		UseCases: map[string]model.UseCase{
			"finish": {
				Name: "Finish", Goal: "Finish", Actor: "User",
				Starts: []model.Snapshot{{State: "start"}},
				End:    map[string]any{"eq": []any{map[string]any{"var": "$state"}, "done"}},
				Scope:  []string{"advance"}, Scenarios: map[string]model.Scenario{},
			},
		},
	}
	if err := os.WriteFile(filepath.Join(root, "project.json"), []byte(model.JSON(project)), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "model.json"), []byte(model.JSON(doc)), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(filepath.Join(root, "project.json"), JSONCodec())
	if err != nil {
		t.Fatal(err)
	}
	for _, diagnostic := range Validate(s) {
		if diagnostic.Severity == "error" {
			t.Fatalf("event-only transition rejected: %+v", diagnostic)
		}
	}
	start, err := s.Normalize(model.Snapshot{State: "start"})
	if err != nil {
		t.Fatal(err)
	}
	next, _, err := engine.New(s).Apply(start, "advance", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next.State != "done" {
		t.Fatalf("event-only transition ended in %q; want done", next.State)
	}
}
