package compiler

import (
	"github.com/nuitsjp/ucspec/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectDuplicateAndTrailingJSON(t *testing.T) {
	for _, x := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, `{} {}`, `[1`} {
		if err := rejectDuplicateKeys([]byte(x)); err == nil {
			t.Errorf("accepted %s", x)
		}
	}
	if err := rejectDuplicateKeys([]byte(`{"a":[{"b":1},{"b":2}]}`)); err != nil {
		t.Fatal(err)
	}
}
func TestUnknownFields(t *testing.T) {
	var p model.Project
	err := decode(JSONCodec(), []byte(`{"schemaVersion":"1","typo":true}`), "x.json", "project", &p)
	if err == nil {
		t.Fatal("unknown field accepted")
	}
}
func TestCompilerFileEscape(t *testing.T) {
	d := t.TempDir()
	p := model.Project{SchemaVersion: "1", Name: "test", Files: []string{"../outside.json"}}
	os.WriteFile(filepath.Join(d, "project.json"), []byte(model.JSON(p)), 0600)
	if _, err := Load(filepath.Join(d, "project.json"), JSONCodec()); err == nil {
		t.Fatal("escape accepted")
	}
}

func TestLoadRetainsProjectOnSourceFailure(t *testing.T) {
	root := t.TempDir()
	p := model.Project{SchemaVersion: "1", Name: "test", Files: []string{"missing.json"}, Output: "custom-report"}
	path := filepath.Join(root, "project.json")
	if err := os.WriteFile(path, []byte(model.JSON(p)), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path, JSONCodec())
	if err == nil || s == nil || s.Project.Output != p.Output {
		t.Fatalf("project configuration lost on source failure: spec=%v error=%v", s, err)
	}
}
func TestMergeDoesNotOverwrite(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument(), Sources: map[string]model.Source{}}
	d := model.EmptyDocument()
	d.States["a"] = model.State{Label: "A"}
	if err := merge(s, d, "a.json"); err != nil {
		t.Fatal(err)
	}
	if err := merge(s, d, "b.json"); err == nil {
		t.Fatal("duplicate accepted")
	}
	if s.Sources["states/a"].File != "a.json" {
		t.Fatal("source overwritten")
	}
}
func TestRequiredReferences(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument(), Project: model.Project{SchemaVersion: "1", Name: "x", Initial: []model.Snapshot{{State: "missing"}}}}
	s.Transitions["broken"] = model.Transition{From: "a", To: "b", Event: "e", Action: "act"}
	ds := Validate(s)
	errors := 0
	for _, d := range ds {
		if d.Severity == "error" {
			errors++
		}
	}
	if errors == 0 {
		t.Fatal("invalid graph accepted")
	}
}

func TestCompoundPredicateCannotHideCoverageAtoms(t *testing.T) {
	x := map[string]any{"not": map[string]any{"and": []any{true, false}}}
	if atomicShape(x) {
		t.Fatal("hidden composite accepted")
	}
	if !atomicShape(map[string]any{"eq": []any{map[string]any{"var": "role"}, "editor"}}) {
		t.Fatal("ordinary predicate rejected")
	}
}

func hierarchicalSpec() *model.Spec {
	s := &model.Spec{Document: model.EmptyDocument(), Sources: map[string]model.Source{}}
	s.Project = model.Project{SchemaVersion: "1", Name: "hierarchy", Files: []string{"model.json"}, Output: "artifacts", Initial: []model.Snapshot{{State: "customer"}}}
	s.Project.Limits.Defaults()
	s.Screens["customer"] = model.Screen{Label: "Customer"}
	s.States["customer"] = model.State{Label: "Customer", Screen: "customer", Events: []string{"close"}, Initial: "panel"}
	s.States["panel"] = model.State{Label: "Panel", Screen: "customer", Events: []string{}, Parent: "customer", Initial: "viewing"}
	s.States["viewing"] = model.State{Label: "Viewing", Screen: "customer", Events: []string{"ask"}, Parent: "panel"}
	s.States["confirm"] = model.State{Label: "Confirm", Screen: "customer", Events: []string{"cancel"}, Parent: "customer"}
	s.States["done"] = model.State{Label: "Done", Screen: "customer", Events: []string{}, Terminal: true}
	for _, id := range []string{"close", "ask", "cancel"} {
		s.Events[id] = model.Event{Label: id, Source: "user"}
	}
	s.Transitions["close"] = model.Transition{From: "customer", To: "done", Event: "close", Action: "Close customer"}
	s.Transitions["ask"] = model.Transition{From: "viewing", To: "confirm", Event: "ask", Action: "Ask for confirmation"}
	s.Transitions["cancel"] = model.Transition{From: "confirm", To: "viewing", Event: "cancel", Action: "Cancel"}
	s.UseCases["customer"] = model.UseCase{Name: "Customer", Goal: "Close customer", Actor: "user", Starts: []model.Snapshot{{State: "customer"}}, End: map[string]any{"eq": []any{map[string]any{"var": "$state"}, "done"}}, Scope: []string{"close", "ask", "cancel"}, Scenarios: map[string]model.Scenario{}}
	return s
}

func TestHierarchyValidation(t *testing.T) {
	for _, d := range Validate(hierarchicalSpec()) {
		if d.Severity == "error" {
			t.Fatalf("valid nested hierarchy rejected: %+v", d)
		}
	}
	cases := []struct {
		name, state, message string
		change               func(*model.State)
	}{
		{"unknown parent", "confirm", "unknown parent state missing", func(st *model.State) { st.Parent = "missing" }},
		{"self parent", "customer", "state cannot be its own parent", func(st *model.State) { st.Parent = "customer" }},
		{"parent cycle", "customer", "parent hierarchy contains a cycle", func(st *model.State) { st.Parent = "viewing" }},
		{"missing initial", "customer", "composite state requires an initial direct child", func(st *model.State) { st.Initial = "" }},
		{"sibling initial", "panel", "initial state must be a direct child", func(st *model.State) { st.Initial = "confirm" }},
		{"self initial", "customer", "initial state must be a direct child", func(st *model.State) { st.Initial = "customer" }},
		{"grandchild initial", "customer", "initial state must be a direct child", func(st *model.State) { st.Initial = "viewing" }},
		{"unknown initial", "customer", "unknown initial state missing", func(st *model.State) { st.Initial = "missing" }},
		{"leaf initial", "viewing", "leaf state must not define initial", func(st *model.State) { st.Initial = "confirm" }},
		{"terminal composite", "customer", "composite state must not be terminal", func(st *model.State) { st.Terminal = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := hierarchicalSpec()
			st := s.States[tc.state]
			tc.change(&st)
			s.States[tc.state] = st
			for _, d := range Validate(s) {
				if d.Subject == "states/"+tc.state && d.Severity == "error" && strings.Contains(d.Message, tc.message) {
					return
				}
			}
			t.Fatalf("missing hierarchy diagnostic %q", tc.message)
		})
	}
}

func TestNegativeStateLayoutLayerRejected(t *testing.T) {
	s := hierarchicalSpec()
	layer := -1
	st := s.States["customer"]
	st.LayoutLayer = &layer
	s.States["customer"] = st
	for _, d := range Validate(s) {
		if d.Subject == "states/customer" && d.Severity == "error" && strings.Contains(d.Message, "layoutLayer must be non-negative") {
			return
		}
	}
	t.Fatal("negative state layout layer accepted")
}

func TestDecisionTransitionMatchesActiveHierarchy(t *testing.T) {
	for _, tc := range []struct {
		name, state, event, transition string
		valid                          bool
	}{
		{"ancestor source", "confirm", "close", "close", true},
		{"composite entry reaches leaf", "customer", "ask", "ask", true},
		{"inactive sibling source", "panel", "cancel", "cancel", false},
		{"different event", "confirm", "cancel", "close", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := hierarchicalSpec()
			s.Decisions["table"] = model.DecisionTable{State: tc.state, Event: tc.event, Cases: []model.DecisionCase{{Enabled: []string{tc.transition}}}}
			for _, d := range Validate(s) {
				if d.Subject == "decisions/table" && strings.Contains(d.Message, "expected transition does not match table state/event") {
					if tc.valid {
						t.Fatalf("active hierarchy rejected: %+v", d)
					}
					return
				}
			}
			if !tc.valid {
				t.Fatal("inactive transition accepted")
			}
		})
	}
}

func TestAncestorTransitionRequiresDirectEventDeclaration(t *testing.T) {
	s := hierarchicalSpec()
	st := s.States["customer"]
	st.Events = []string{}
	s.States["customer"] = st
	st = s.States["viewing"]
	st.Events = append(st.Events, "close")
	s.States["viewing"] = st
	for _, d := range Validate(s) {
		if d.Subject == "transitions/close" && d.Message == "event not declared on source state" {
			return
		}
	}
	t.Fatal("child event declaration was accepted for an ancestor transition")
}
