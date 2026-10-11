package engine

import (
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
	testkit "github.com/nuitsjp/ucspec/internal/testkit/choicefixture"
)

func TestChoiceUsesIncomingEffectsAndDoesNotAcceptExternalEvents(t *testing.T) {
	s := testkit.ChoiceSpec()
	e := New(s)
	start, _ := s.Normalize(model.Snapshot{State: "ready"})
	choice, _, err := e.Apply(start, "search", map[string]any{"found": true})
	if err != nil || choice.State != "result" || !model.Equal(choice.Values["matched"], true) {
		t.Fatalf("incoming effect: %+v, %v", choice, err)
	}
	ids, obs, available, err := e.Select(choice, "", nil)
	if err != nil || !available || !model.Equal(ids, []string{"found"}) || len(obs) != 2 {
		t.Fatalf("dynamic branch: %v, %+v, %v", ids, obs, err)
	}
	if ids, _, available, err := e.Select(choice, "search", map[string]any{"found": false}); err != nil || available || len(ids) != 0 {
		t.Fatalf("choice accepted external event: %v, %v, %v", ids, available, err)
	}
	if _, _, _, err := e.Select(choice, "", map[string]any{"found": true}); err == nil {
		t.Fatal("branch accepted external parameters")
	}
	if _, _, err := e.Apply(choice, "empty", nil); err == nil {
		t.Fatal("wrong branch accepted")
	}
	end, _, err := e.Apply(choice, "found", nil)
	if err != nil || end.State != "done" || !model.Equal(end.Values["alert"], false) {
		t.Fatalf("branch result: %+v, %v", end, err)
	}
}

func TestChoiceReplayCannotFinishOrStartAtTransientVertex(t *testing.T) {
	s := testkit.ChoiceSpec()
	e := New(s)
	u := s.UseCases["search"]
	for id, scenario := range u.Scenarios {
		if r := e.Replay(id, scenario, &u); !r.Valid || len(r.Trace) != 3 {
			t.Fatal(r)
		}
	}
	incomplete := model.Scenario{Name: "Incomplete", Goal: "Find matching records", ExpectedOutcome: "The result branch resolves", Start: model.Snapshot{State: "ready"}, Steps: []model.Step{{Transition: "search", Input: map[string]any{"found": true}}}, Expect: true}
	if r := e.Replay("incomplete", incomplete, nil); r.Valid {
		t.Fatal("regression completed at choice")
	}
	incomplete.Start.State = "result"
	if r := e.Replay("start", incomplete, nil); r.Valid {
		t.Fatal("scenario started at choice")
	}
	// The incoming effect already meets this predicate at the choice, but the
	// eventless branch must still execute before a use case can finish.
	u.End = map[string]any{"var": "matched"}
	if r := e.Replay("found", u.Scenarios["found"], &u); !r.Valid {
		t.Fatal(r)
	}
}

func TestChoiceBlocksInheritedEventsAndTraversesNestedChoices(t *testing.T) {
	s := testkit.ChoiceSpec()
	s.States["workspace"] = model.State{Label: "Workspace", Screen: "screen", Initial: "ready", Events: []string{"search"}}
	ready := s.States["ready"]
	ready.Parent = "workspace"
	s.States["ready"] = ready
	s.Choices["result"] = model.Choice{Label: "Result", Parent: "workspace"}
	s.Choices["second"] = model.Choice{Label: "Second", Parent: "workspace"}
	transition := s.Transitions["found"]
	transition.To = "second"
	s.Transitions["found"] = transition
	s.Transitions["finish"] = model.Transition{From: "second", To: "done", Guard: true, Action: "Finish"}
	e := New(s)
	st, _ := s.Normalize(model.Snapshot{State: "ready"})
	st, _, _ = e.Apply(st, "search", map[string]any{"found": true})
	if !s.InState(st.State, "workspace") || !model.Equal(e.Events(st.State), []string{""}) {
		t.Fatal("choice lost containment or inherited an external event")
	}
	st, _, _ = e.Apply(st, "found", nil)
	st, _, err := e.Apply(st, "finish", nil)
	if err != nil || st.State != "done" {
		t.Fatalf("choice chain: %+v, %v", st, err)
	}
}
