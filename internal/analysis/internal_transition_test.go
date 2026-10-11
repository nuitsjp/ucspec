package analysis

import (
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
)

func internalAnalysisSpec(includeInternalScenario bool) *model.Spec {
	s := &model.Spec{Document: model.EmptyDocument()}
	s.Project = model.Project{
		SchemaVersion: "1",
		Name:          "internal transitions",
		Files:         []string{"model.json"},
		Output:        "artifacts",
		Initial:       []model.Snapshot{{State: "workspace"}},
	}
	s.Project.Limits.Defaults()
	s.Screens["workspace"] = model.Screen{Label: "Workspace"}
	s.States["workspace"] = model.State{Label: "Workspace", Screen: "workspace", Initial: "ready", Events: []string{"openAlternate", "refresh", "reset", "finish"}}
	s.States["ready"] = model.State{Label: "Ready", Screen: "workspace", Parent: "workspace"}
	s.States["alternate"] = model.State{Label: "Alternate", Screen: "workspace", Parent: "workspace"}
	s.States["done"] = model.State{Label: "Done", Screen: "workspace", Terminal: true}
	for _, id := range []string{"openAlternate", "refresh", "reset", "finish"} {
		s.Events[id] = model.Event{Label: id, Source: "user"}
	}
	s.Variables["revision"] = model.Variable{Domain: []any{0, 1}, Initial: 0}
	s.Transitions["openAlternate"] = model.Transition{From: "workspace", To: "alternate", Event: "openAlternate", Action: "Open alternate"}
	s.Transitions["refresh"] = model.Transition{
		From: "workspace", To: "workspace", Kind: "internal", Event: "refresh", Action: "Refresh",
		Set: map[string]model.Expr{"revision": 1},
	}
	s.Transitions["reset"] = model.Transition{From: "workspace", To: "workspace", Event: "reset", Action: "Reset"}
	s.Transitions["finish"] = model.Transition{From: "workspace", To: "done", Event: "finish", Action: "Finish"}
	scope := []string{"openAlternate", "refresh", "reset", "finish"}
	steps := []model.Step{
		{Transition: "openAlternate"},
	}
	if includeInternalScenario {
		steps = append(steps, model.Step{Transition: "refresh"})
	}
	steps = append(steps, model.Step{Transition: "reset"}, model.Step{Transition: "finish"})
	s.UseCases["workspace"] = model.UseCase{
		Name: "Workspace", Goal: "Finish workspace", Actor: "User",
		Starts: []model.Snapshot{{State: "workspace"}},
		End:    map[string]any{"eq": []any{map[string]any{"var": "$state"}, "done"}},
		Scope:  scope,
		Scenarios: map[string]model.Scenario{
			"flow": {Name: "Flow", Goal: "Complete work after switching the view", ExpectedOutcome: "The workspace resets and finishes", Start: model.Snapshot{State: "workspace"}, Steps: steps, Expect: map[string]any{"eq": []any{map[string]any{"var": "$state"}, "done"}}},
		},
	}
	return s
}

func coverageOf(r model.UseCaseResult, kind string) (model.Coverage, bool) {
	for _, c := range r.Coverage {
		if c.Kind == kind {
			return c, true
		}
	}
	return model.Coverage{}, false
}

func requirementContains(reqs []model.Requirement, id string) bool {
	for _, req := range reqs {
		if req.ID == id {
			return true
		}
	}
	return false
}

func pathContains(reqs []model.Requirement, id string) bool {
	for _, req := range reqs {
		for _, transition := range req.Path {
			if transition == id {
				return true
			}
		}
	}
	return false
}

func TestInternalTransitionIsReplayedAndCovered(t *testing.T) {
	r := Analyze(internalAnalysisSpec(true), "test")
	if r.Status == "failed" {
		t.Fatalf("analysis failed: %+v", r.Diagnostics)
	}
	if len(r.Scenarios) != 1 || !r.Scenarios[0].Valid {
		t.Fatalf("scenario did not replay: %+v", r.Scenarios)
	}
	found := false
	for _, step := range r.Scenarios[0].Witness.Steps {
		if step.Transition == "refresh" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scenario witness omitted internal transition: %+v", r.Scenarios[0].Witness)
	}
	if len(r.UseCases) != 1 {
		t.Fatalf("unexpected use-case results: %+v", r.UseCases)
	}
	transitions, ok := coverageOf(r.UseCases[0], "transitions")
	if !ok || transitions.Total != 4 || transitions.Covered != 4 {
		t.Fatalf("transition coverage does not include internal ID: %+v", transitions)
	}
}

func TestUncoveredInternalTransitionProducesCandidatePaths(t *testing.T) {
	r := Analyze(internalAnalysisSpec(false), "test")
	if r.Status == "failed" {
		t.Fatalf("analysis failed: %+v", r.Diagnostics)
	}
	if len(r.UseCases) != 1 {
		t.Fatalf("unexpected use-case results: %+v", r.UseCases)
	}
	transitions, ok := coverageOf(r.UseCases[0], "transitions")
	if !ok || !requirementContains(transitions.Missing, "refresh") {
		t.Fatalf("uncovered internal transition was not a candidate requirement: %+v", transitions)
	}
	prime, ok := coverageOf(r.UseCases[0], "primePaths")
	if !ok || !pathContains(prime.Missing, "refresh") {
		t.Fatalf("prime-path candidates omitted internal transition: %+v", prime)
	}
}
