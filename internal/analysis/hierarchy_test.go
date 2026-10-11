package analysis

import (
	"fmt"
	"testing"

	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
)

func hierarchicalSpec() *model.Spec {
	s := &model.Spec{Document: model.EmptyDocument(), Project: model.Project{
		SchemaVersion: "1", Name: "Nested deletion", Files: []string{"model.json"}, Output: "artifacts",
		Initial: []model.Snapshot{{State: "outside"}}, Coverage: map[string]float64{},
	}}
	s.Project.Limits.Defaults()
	s.Screens["customer"] = model.Screen{Label: "Customer"}
	s.States = map[string]model.State{
		"outside":    {Label: "Outside", Screen: "customer", Events: []string{"open"}},
		"view":       {Label: "View", Screen: "customer", Initial: "viewing", Events: []string{"leave"}},
		"viewing":    {Label: "Normal", Screen: "customer", Parent: "view", Events: []string{"delete"}},
		"confirming": {Label: "Confirm", Screen: "customer", Parent: "view", Events: []string{"confirm", "cancel"}},
		"deleting":   {Label: "Deleting", Screen: "customer", Parent: "view", Events: []string{"result"}},
	}
	for _, id := range []string{"open", "leave", "delete", "confirm", "cancel", "result"} {
		s.Events[id] = model.Event{Label: id, Source: "user"}
	}
	s.Events["result"] = model.Event{Label: "result", Source: "system", Parameters: map[string][]any{"success": {false, true}}}
	s.Conditions["success"] = map[string]any{"param": "success"}
	s.Transitions = map[string]model.Transition{
		"enter":   {From: "outside", To: "view", Event: "open", Action: "Open", UnownedReason: "Setup"},
		"leave":   {From: "view", To: "outside", Event: "leave", Action: "Leave"},
		"begin":   {From: "viewing", To: "confirming", Event: "delete", Action: "Ask"},
		"confirm": {From: "confirming", To: "deleting", Event: "confirm", Action: "Start"},
		"cancel":  {From: "confirming", To: "view", Event: "cancel", Action: "Cancel"},
		"success": {From: "deleting", To: "outside", Event: "result", Action: "Done", Guard: map[string]any{"condition": "success"}},
		"failure": {From: "deleting", To: "view", Event: "result", Action: "Retry", Guard: map[string]any{"not": map[string]any{"condition": "success"}}},
	}
	s.UseCases["delete"] = model.UseCase{
		Name: "Delete", Goal: "Delete or cancel", Actor: "Editor", Starts: []model.Snapshot{{State: "view"}},
		End: map[string]any{"inState": "outside"}, Scope: []string{"leave", "begin", "confirm", "cancel", "success", "failure"},
		Scenarios: map[string]model.Scenario{"leave": {Name: "Leave without deleting", Goal: "Keep the customer and leave", ExpectedOutcome: "The customer view closes without deletion", Start: model.Snapshot{State: "view"}, Steps: []model.Step{{Transition: "leave"}}, Expect: map[string]any{"inState": "outside"}}},
	}
	return s
}

func findCoverage(t *testing.T, r *model.Report, kind string) model.Coverage {
	t.Helper()
	for _, c := range r.UseCases[0].Coverage {
		if c.Kind == kind {
			return c
		}
	}
	t.Fatalf("coverage %s absent", kind)
	return model.Coverage{}
}

func TestHierarchyCoverageDoesNotHideUnvisitedChildren(t *testing.T) {
	s := hierarchicalSpec()
	r := Analyze(s, "test")
	if r.Status != "verified" {
		t.Fatalf("%s: %+v", r.Status, r.Diagnostics)
	}
	for _, d := range r.Diagnostics {
		if d.Code == "unreachable-state" && d.Subject == "states/view" {
			t.Fatal("active parent reported unreachable")
		}
	}
	leaves := findCoverage(t, r, "states")
	parents := findCoverage(t, r, "compositeStates")
	if leaves.Covered != 2 || leaves.Total != 4 || !leaves.Complete || parents.Covered != 1 || parents.Total != 1 {
		t.Fatalf("leaves=%+v parents=%+v", leaves, parents)
	}
	missing := map[string]bool{}
	for _, req := range leaves.Missing {
		missing[req.State] = true
		if req.Witness == nil {
			t.Fatalf("missing completion witness: %+v", req)
		}
		u := s.UseCases["delete"]
		sc := model.Scenario{Name: "Candidate", Goal: "Exercise the uncovered state", ExpectedOutcome: "The use case completes after visiting the state", Start: req.Witness.Start, Steps: req.Witness.Steps, Expect: u.End}
		result := engine.New(s).Replay("candidate", sc, &u)
		if !result.Valid {
			t.Fatalf("invalid state candidate: %s", result.Error)
		}
		visited := false
		for _, st := range result.Trace {
			visited = visited || s.InState(st.State, req.State)
		}
		if !visited {
			t.Fatalf("candidate does not visit %s", req.State)
		}
	}
	if !missing["confirming"] || !missing["deleting"] || len(missing) != 2 {
		t.Fatal(missing)
	}
	transitions := findCoverage(t, r, "transitions")
	if transitions.Total != 6 || transitions.Covered != 1 {
		t.Fatalf("declared parent edge counted more than once: %+v", transitions)
	}
	u := s.UseCases["delete"]
	u.Scenarios["success"] = model.Scenario{Name: "Delete", Goal: "Delete the customer", ExpectedOutcome: "Deletion completes and closes the customer view", Start: model.Snapshot{State: "view"}, Steps: []model.Step{
		{Transition: "begin"}, {Transition: "confirm"}, {Transition: "success", Input: map[string]any{"success": true}},
	}, Expect: u.End}
	s.UseCases["delete"] = u
	if c := findCoverage(t, Analyze(s, "test"), "states"); c.Covered != 4 || c.Total != 4 {
		t.Fatal(c)
	}
}

func TestHierarchyPathsIncludeDefaultEntryAndInheritedExit(t *testing.T) {
	s := hierarchicalSpec()
	r := Analyze(s, "test")
	pairs := findCoverage(t, r, "pairs")
	ids := map[string]bool{}
	for _, req := range pairs.Missing {
		ids[req.ID] = true
	}
	for _, expected := range []string{"begin > leave", "confirm > leave", "cancel > begin", "failure > begin"} {
		if !ids[expected] {
			t.Fatalf("semantic connection %s missing from %+v", expected, pairs)
		}
	}
	scope := map[string]bool{}
	for _, id := range s.UseCases["delete"].Scope {
		scope[id] = true
	}
	paths, complete := PrimePaths(s, scope, 10000)
	found := false
	for _, path := range paths {
		found = found || containsPath(path, []string{"begin", "confirm", "leave"})
	}
	if !complete || !found {
		t.Fatalf("inherited exit missing from prime paths: %v, complete=%v", paths, complete)
	}
}

func TestHierarchyScopeCannotBypassChildPriority(t *testing.T) {
	s := hierarchicalSpec()
	st := s.States["viewing"]
	st.Events = append(st.Events, "leave")
	s.States["viewing"] = st
	s.Transitions["childLeave"] = model.Transition{From: "viewing", To: "outside", Event: "leave", Action: "Specific exit", UnownedReason: "Outside the chosen scope"}
	u := s.UseCases["delete"]
	u.Scope = []string{"leave"}
	g := Explore(engine.New(s), u.Starts, map[string]bool{"leave": true}, u.End)
	if !g.Complete || len(g.Edges) != 0 {
		t.Fatalf("scope bypassed enabled child transition: %+v", g)
	}
}

func TestHierarchyStateCoverageIncompleteAndInvalidScenarios(t *testing.T) {
	s := hierarchicalSpec()
	u := s.UseCases["delete"]
	sc := u.Scenarios["leave"]
	sc.Expect = false
	u.Scenarios["leave"] = sc
	s.UseCases["delete"] = u
	r := Analyze(s, "test")
	for _, kind := range []string{"states", "compositeStates"} {
		if c := findCoverage(t, r, kind); c.Covered != 0 {
			t.Fatalf("invalid scenario covered %s: %+v", kind, c)
		}
	}
	s.Project.Limits.States = 1
	r = Analyze(s, "test")
	for _, kind := range []string{"states", "compositeStates"} {
		if c := findCoverage(t, r, kind); c.Complete {
			t.Fatalf("incomplete exploration claimed complete %s", kind)
		}
	}
}

func TestHierarchyStateThresholdAndParentChangeImpact(t *testing.T) {
	s := hierarchicalSpec()
	s.Project.Coverage["states"] = 100
	if r := Analyze(s, "test"); r.Status != "failed" {
		t.Fatalf("missing children passed state threshold: %s", r.Status)
	}
	delete(s.Project.Coverage, "states")
	old := Analyze(s, "test")
	parent := s.States["view"]
	parent.Initial = "confirming"
	s.States["view"] = parent
	now := Analyze(s, "test")
	if err := Compare(now, old); err != nil {
		t.Fatal(err)
	}
	if !model.Contains(now.Impact.AffectedUseCases, "delete") {
		t.Fatalf("parent entry change omitted from impact: %+v", now.Impact)
	}
}

func TestHierarchyImpactIncludesInheritedApproachToUseCaseStart(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument(), Project: model.Project{
		SchemaVersion: "1", Name: "hierarchical approach", Files: []string{"model.json"}, Output: "out",
		Initial: []model.Snapshot{{State: "boot"}},
	}}
	s.Project.Limits.Defaults()
	s.Screens["screen"] = model.Screen{Label: "screen"}
	s.States = map[string]model.State{
		"boot":    {Label: "boot", Screen: "screen", Events: []string{"go"}},
		"staging": {Label: "staging", Screen: "screen", Initial: "gate", Events: []string{"go"}},
		"gate":    {Label: "gate", Screen: "screen", Parent: "staging"},
		"work":    {Label: "work", Screen: "screen", Initial: "ready", Events: []string{"go"}},
		"ready":   {Label: "ready", Screen: "screen", Parent: "work"},
		"done":    {Label: "done", Screen: "screen", Terminal: true},
		"stopped": {Label: "stopped", Screen: "screen", Terminal: true},
	}
	s.Events["go"] = model.Event{Label: "go", Source: "user"}
	s.Transitions["enter"] = model.Transition{From: "boot", To: "gate", Event: "go", Action: "enter", UnownedReason: "before flow"}
	s.Transitions["prepare"] = model.Transition{From: "staging", To: "work", Event: "go", Action: "prepare", UnownedReason: "before flow"}
	s.Transitions["finish"] = model.Transition{From: "work", To: "done", Event: "go", Action: "finish"}
	s.UseCases["flow"] = model.UseCase{
		Name: "flow", Goal: "finish", Actor: "actor", Starts: []model.Snapshot{{State: "work"}}, Scope: []string{"finish"},
		End: map[string]any{"inState": "done"},
	}
	before := Analyze(s, "test")
	if before.Status != "verified" {
		t.Fatalf("invalid baseline: %+v", before.Diagnostics)
	}
	enter := s.Transitions["enter"]
	enter.To = "stopped"
	s.Transitions["enter"] = enter
	after := Analyze(s, "test")
	unreachable := false
	for _, diagnostic := range after.Diagnostics {
		unreachable = unreachable || diagnostic.Code == "unreachable-start" && diagnostic.Subject == "usecases/flow"
	}
	if after.Status != "failed" || !unreachable {
		t.Fatalf("approach change did not invalidate the start: %+v", after.Diagnostics)
	}
	if err := Compare(after, before); err != nil {
		t.Fatal(err)
	}
	if !model.Equal(after.Impact.AffectedUseCases, []string{"flow"}) {
		t.Fatalf("inherited approach change omitted from impact: %+v", after.Impact)
	}
}

func TestHierarchicalCompletionWitnessesPreserveStartAndChoiceSteps(t *testing.T) {
	s := convergingStartsSpec()
	u := s.UseCases["flow"]
	for i, leafID := range []string{"a", "b"} {
		parentID := "parent" + leafID
		leaf := s.States[leafID]
		s.States[parentID] = model.State{Label: parentID, Screen: "screen", Initial: leafID, Events: leaf.Events}
		leaf.Parent = parentID
		leaf.Events = nil
		s.States[leafID] = leaf
		transition := s.Transitions["t"+leafID]
		transition.From = parentID
		s.Transitions["t"+leafID] = transition
		u.Starts[i].State = parentID
	}
	s.Choices["result"] = model.Choice{Label: "Result"}
	finish := s.Transitions["finish"]
	finish.To = "result"
	s.Transitions["finish"] = finish
	s.Transitions["resolve"] = model.Transition{From: "result", To: "end", Guard: true, Action: "Complete"}
	u.Scope = append(u.Scope, "resolve")
	fromB := u.Scenarios["fromB"]
	fromB.Steps = append(fromB.Steps, model.Step{Transition: "resolve"})
	u.Scenarios["fromB"] = fromB
	s.UseCases["flow"] = u
	r := Analyze(s, "test")
	if r.Status != "verified" {
		t.Fatalf("invalid hierarchy and choice model: %+v", r.Diagnostics)
	}
	for i, scenarioID := range []string{"fromA", "fromB"} {
		id := fmt.Sprintf("completion/flow/%d", i+1)
		found := false
		for _, check := range r.Checks {
			if check.ID != id {
				continue
			}
			found = true
			want := u.Scenarios[scenarioID]
			if check.Status != "verified" || check.Witness == nil || check.Witness.Start.State != want.Start.State || !model.Equal(check.Witness.Steps, want.Steps) {
				t.Fatalf("wrong witness for %s: %+v", id, check)
			}
			want.Start, want.Steps = check.Witness.Start, check.Witness.Steps
			if replay := engine.New(s).Replay(id, want, &u); !replay.Valid {
				t.Fatalf("hierarchical completion witness cannot replay: %s", replay.Error)
			}
		}
		if !found {
			t.Fatalf("missing completion check %s", id)
		}
	}
}
