package compiler

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/model"
	testkit "github.com/nuitsjp/ucspec/internal/testkit/choicefixture"
)

func TestChoiceStructureValidation(t *testing.T) {
	if diagnostics := Validate(testkit.ChoiceSpec()); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	tests := []struct {
		name    string
		mutate  func(*model.Spec)
		message string
	}{
		{"duplicate", func(s *model.Spec) { s.Choices["ready"] = model.Choice{Label: "Duplicate"} }, "duplicates a state"},
		{"unknown target", func(s *model.Spec) { tr := s.Transitions["search"]; tr.To = "unknown"; s.Transitions["search"] = tr }, "unknown to vertex"},
		{"event on branch", func(s *model.Spec) { tr := s.Transitions["found"]; tr.Event = "search"; s.Transitions["found"] = tr }, "must not have an event"},
		{"no guard", func(s *model.Spec) { tr := s.Transitions["found"]; tr.Guard = nil; s.Transitions["found"] = tr }, "requires a guard"},
		{"parameter on branch", func(s *model.Spec) {
			tr := s.Transitions["found"]
			tr.Set = map[string]any{"matched": map[string]any{"param": "found"}}
			s.Transitions["found"] = tr
		}, "unavailable parameter"},
		{"no branch", func(s *model.Spec) { s.Choices["dangling"] = model.Choice{Label: "Dangling"} }, "outgoing branch"},
		{"leaf parent", func(s *model.Spec) { s.Choices["result"] = model.Choice{Label: "Result", Parent: "ready"} }, "composite state"},
		{"cycle", func(s *model.Spec) { tr := s.Transitions["found"]; tr.To = "result"; s.Transitions["found"] = tr }, "eventless cycle"},
		{"initial", func(s *model.Spec) { s.Project.Initial[0].State = "result" }, "initial configuration must be a state"},
		{"scenario start", func(s *model.Spec) {
			sc := s.UseCases["search"].Scenarios["found"]
			sc.Start.State = "result"
			s.UseCases["search"].Scenarios["found"] = sc
		}, "scenario must start in a state"},
		{"usecase start", func(s *model.Spec) { u := s.UseCases["search"]; u.Starts[0].State = "result"; s.UseCases["search"] = u }, "use-case start must be a state"},
		{"label without guard", func(s *model.Spec) {
			tr := s.Transitions["search"]
			tr.GuardLabel = "Misleading"
			s.Transitions["search"] = tr
		}, "guardLabel requires a guard"},
		{"no state event", func(s *model.Spec) { tr := s.Transitions["search"]; tr.Event = ""; s.Transitions["search"] = tr }, "unknown event"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := testkit.ChoiceSpec()
			test.mutate(s)
			for _, diagnostic := range Validate(s) {
				if diagnostic.Severity == "error" && strings.Contains(diagnostic.Message, test.message) {
					return
				}
			}
			t.Fatalf("missing %q in %+v", test.message, Validate(s))
		})
	}
}

func TestChoiceIndependentTableAndMerge(t *testing.T) {
	s := testkit.ChoiceSpec()
	s.Decisions["branch"] = model.DecisionTable{State: "result", Cases: []model.DecisionCase{{Values: map[string]any{"matched": true}, Enabled: []string{"found"}}}}
	if diagnostics := Validate(s); len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	doc := model.EmptyDocument()
	doc.Choices["other"] = model.Choice{Label: "Other"}
	if err := merge(s, doc, "other.json"); err != nil || s.Sources["choices/other"].Pointer != "/choices/other" {
		t.Fatalf("choice merge: %v, %+v", err, s.Sources)
	}
	if err := merge(s, doc, "duplicate.json"); err == nil {
		t.Fatal("duplicate choice definitions merged")
	}
}
