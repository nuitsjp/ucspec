package model

import (
	"strings"
	"testing"
)

func modelHierarchySpec() *Spec {
	s := &Spec{Document: EmptyDocument()}
	s.Variables["mode"] = Variable{Domain: []any{"default", "explicit"}, Initial: "default"}
	s.States = map[string]State{
		"app":       {Label: "App", Initial: "workspace"},
		"workspace": {Label: "Workspace", Parent: "app", Initial: "ready"},
		"ready":     {Label: "Ready", Parent: "workspace"},
		"alternate": {Label: "Alternate", Parent: "workspace"},
		"outside":   {Label: "Outside"},
	}
	return s
}

func TestNestedCompositeEntryNormalizesToTheDefaultLeaf(t *testing.T) {
	s := modelHierarchySpec()

	if got, err := s.Enter("app"); err != nil || got != "ready" {
		t.Fatalf("Enter(app) = %q, %v; want ready", got, err)
	}
	if got, err := s.Enter("workspace"); err != nil || got != "ready" {
		t.Fatalf("Enter(workspace) = %q, %v; want ready", got, err)
	}
	if got, err := s.Enter("alternate"); err != nil || got != "alternate" {
		t.Fatalf("Enter(alternate) = %q, %v; want alternate", got, err)
	}

	input := Snapshot{State: "app", Values: map[string]any{"mode": "explicit"}}
	normalized, err := s.Normalize(input)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.State != "ready" || !Equal(normalized.Values["mode"], "explicit") {
		t.Fatalf("Normalize(app) = %+v; want ready with the supplied value", normalized)
	}

	explicit, err := s.Normalize(Snapshot{State: "alternate"})
	if err != nil {
		t.Fatal(err)
	}
	if explicit.State != "alternate" {
		t.Fatalf("Normalize(alternate) = %q; explicit child target was replaced", explicit.State)
	}
}

func TestStateQueriesFollowDirectParentsAndContainment(t *testing.T) {
	s := modelHierarchySpec()

	if got := s.StatePath("ready"); !Equal(got, []string{"ready", "workspace", "app"}) {
		t.Fatalf("StatePath(ready) = %v", got)
	}
	for _, id := range []string{"ready", "workspace", "app"} {
		if !s.InState("ready", id) {
			t.Errorf("ready should be in %s", id)
		}
	}
	if s.InState("ready", "outside") {
		t.Fatal("outside incorrectly contains ready")
	}
	if got := s.Children("app"); !Equal(got, []string{"workspace"}) {
		t.Fatalf("Children(app) = %v", got)
	}
	if got := s.Children("workspace"); !Equal(got, []string{"alternate", "ready"}) {
		t.Fatalf("Children(workspace) = %v", got)
	}
	if got := s.Leaves("workspace"); !Equal(got, []string{"alternate", "ready"}) {
		t.Fatalf("Leaves(workspace) = %v", got)
	}
}

func TestHierarchyQueriesDoNotMutateTheSpecificationOrSnapshot(t *testing.T) {
	s := modelHierarchySpec()
	beforeStates := JSON(s.States)
	input := Snapshot{State: "app", Values: map[string]any{"mode": "explicit"}}
	beforeInput := JSON(input)

	if _, err := s.Enter("app"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Normalize(input); err != nil {
		t.Fatal(err)
	}
	_ = s.StatePath("ready")
	_ = s.InState("ready", "workspace")
	_ = s.Children("workspace")
	_ = s.Leaves("workspace")

	if got := JSON(s.States); got != beforeStates {
		t.Fatalf("state definitions changed during queries: before %s, after %s", beforeStates, got)
	}
	if got := JSON(input); got != beforeInput {
		t.Fatalf("input snapshot changed during normalization: before %s, after %s", beforeInput, got)
	}
}

func TestCyclicInitialEntryFailsWithoutChangingTheModel(t *testing.T) {
	s := &Spec{Document: EmptyDocument()}
	s.States["first"] = State{Label: "First", Initial: "second"}
	s.States["second"] = State{Label: "Second", Initial: "first"}
	before := JSON(s.States)

	if _, err := s.Enter("first"); err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("Enter(first) error = %v; want a cyclic initial-state error", err)
	}
	if _, err := s.Normalize(Snapshot{State: "first"}); err == nil {
		t.Fatal("Normalize accepted a cyclic initial state")
	}
	if got := JSON(s.States); got != before {
		t.Fatalf("cyclic-entry check changed the model: before %s, after %s", before, got)
	}
}
