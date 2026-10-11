package choicefixture

import "github.com/nuitsjp/ucspec/internal/model"

// ChoiceSpec supplies an independent model where the incoming effect changes
// the branch result. Reading the original value would choose the wrong path.
func ChoiceSpec() *model.Spec {
	s := &model.Spec{Document: model.EmptyDocument(), Sources: map[string]model.Source{}}
	s.Project = model.Project{SchemaVersion: "1", Name: "Choice", Files: []string{"model.json"}, Output: "artifacts", Initial: []model.Snapshot{{State: "ready"}}, Limits: model.Limits{States: 100, Steps: 8, Paths: 10000, Combinations: 10}}
	s.Variables["matched"] = model.Variable{Domain: []any{false, true}, Initial: false}
	s.Variables["alert"] = model.Variable{Domain: []any{false, true}, Initial: false}
	s.Screens["screen"] = model.Screen{Label: "Search"}
	s.States["ready"] = model.State{Label: "Ready", Screen: "screen", Events: []string{"search"}}
	s.States["done"] = model.State{Label: "Done", Screen: "screen", Terminal: true}
	s.Choices["result"] = model.Choice{Label: "Result"}
	s.Events["search"] = model.Event{Label: "Search button", Source: "user", Parameters: map[string][]any{"found": {false, true}}}
	s.Conditions["hasResult"] = map[string]any{"var": "matched"}
	s.Transitions["search"] = model.Transition{From: "ready", To: "result", Event: "search", Action: "Fetch", Set: map[string]any{"matched": map[string]any{"param": "found"}}}
	s.Transitions["found"] = model.Transition{From: "result", To: "done", Guard: map[string]any{"condition": "hasResult"}, GuardLabel: "Found", Action: "Display", Set: map[string]any{"alert": false}}
	s.Transitions["empty"] = model.Transition{From: "result", To: "done", Guard: map[string]any{"not": map[string]any{"condition": "hasResult"}}, GuardLabel: "Empty", Action: "Alert", Set: map[string]any{"alert": true}}
	s.UseCases["search"] = model.UseCase{Name: "Search", Goal: "See results", Actor: "Staff", Starts: []model.Snapshot{{State: "ready"}}, Scope: []string{"search", "found", "empty"}, End: map[string]any{"eq": []any{map[string]any{"var": "$state"}, "done"}}, Scenarios: map[string]model.Scenario{
		"found": {Name: "Found", Goal: "Find matching records", ExpectedOutcome: "Results appear without an alert", Start: model.Snapshot{State: "ready"}, Steps: []model.Step{{Transition: "search", Input: map[string]any{"found": true}}, {Transition: "found"}}, Expect: map[string]any{"eq": []any{map[string]any{"var": "alert"}, false}}},
		"empty": {Name: "Empty", Goal: "Understand why no records appear", ExpectedOutcome: "An alert explains the empty result", Start: model.Snapshot{State: "ready"}, Steps: []model.Step{{Transition: "search", Input: map[string]any{"found": false}}, {Transition: "empty"}}, Expect: map[string]any{"eq": []any{map[string]any{"var": "alert"}, true}}},
	}}
	return s
}
