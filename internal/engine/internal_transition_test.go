package engine_test

import (
	"testing"

	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
)

func internalTransitionHierarchy() *model.Spec {
	s := &model.Spec{Document: model.EmptyDocument()}
	s.Variables["revision"] = model.Variable{Domain: []any{0, 1}, Initial: 0}
	s.States["workspace"] = model.State{Initial: "ready", Events: []string{"refresh", "reset"}}
	s.States["ready"] = model.State{Parent: "workspace"}
	s.States["editing"] = model.State{Parent: "workspace"}
	s.Events["refresh"] = model.Event{Label: "Refresh", Source: "user"}
	s.Events["reset"] = model.Event{Label: "Reset", Source: "user"}
	s.Transitions["refresh"] = model.Transition{
		From: "workspace", To: "workspace", Kind: "internal", Event: "refresh", Action: "Refresh",
		Set: map[string]model.Expr{"revision": 1},
	}
	s.Transitions["reset"] = model.Transition{
		From: "workspace", To: "workspace", Event: "reset", Action: "Reset",
	}
	return s
}

func TestInternalCompositeTransitionPreservesActiveChildAndAppliesSet(t *testing.T) {
	s := internalTransitionHierarchy()
	start, err := s.Normalize(model.Snapshot{State: "editing"})
	if err != nil {
		t.Fatal(err)
	}
	next, _, err := engine.New(s).Apply(start, "refresh", nil)
	if err != nil {
		t.Fatal(err)
	}
	if next.State != "editing" {
		t.Fatalf("internal transition entered %q; want the active child editing", next.State)
	}
	if !model.Equal(next.Values["revision"], 1) {
		t.Fatalf("internal transition did not apply its assignment: %+v", next.Values)
	}
}

func TestExternalCompositeSelfTransitionReentersInitialChild(t *testing.T) {
	for _, kind := range []string{"", "external"} {
		t.Run(kind, func(t *testing.T) {
			s := internalTransitionHierarchy()
			reset := s.Transitions["reset"]
			reset.Kind = kind
			s.Transitions["reset"] = reset
			start, err := s.Normalize(model.Snapshot{State: "editing"})
			if err != nil {
				t.Fatal(err)
			}
			next, _, err := engine.New(s).Apply(start, "reset", nil)
			if err != nil {
				t.Fatal(err)
			}
			if next.State != "ready" {
				t.Fatalf("external self transition stayed in %q; want initial child ready", next.State)
			}
		})
	}
}
