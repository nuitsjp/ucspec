package analysis

import (
	"testing"

	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
	testkit "github.com/nuitsjp/ucspec/internal/testkit/choicefixture"
)

func TestChoiceBranchesCountInCoverageButNotStateCoverage(t *testing.T) {
	s := testkit.ChoiceSpec()
	u := s.UseCases["search"]
	delete(u.Scenarios, "empty")
	s.UseCases["search"] = u
	r := Analyze(s, "test")
	if r.Status != "verified" || r.ReachableStates != 3 {
		t.Fatalf("stable configurations: %s, %d, %+v", r.Status, r.ReachableStates, r.Diagnostics)
	}
	for _, c := range r.UseCases[0].Coverage {
		switch c.Kind {
		case "states":
			if c.Total != 2 || c.Covered != 2 {
				t.Fatal(c)
			}
		case "transitions":
			if c.Total != 3 || c.Covered != 2 || len(c.Missing) != 1 || c.Missing[0].ID != "empty" {
				t.Fatal(c)
			}
			if c.Missing[0].Witness == nil || len(c.Missing[0].Witness.Steps) != 2 {
				t.Fatal("missing branch has no complete witness")
			}
		case "pairs", "primePaths", "boundedPaths":
			if c.Total != 2 || c.Covered != 1 {
				t.Fatal(c)
			}
		case "conditions", "multipleConditions":
			if c.Total != 4 || c.Covered != 2 {
				t.Fatal(c)
			}
		}
	}
	if !model.Contains(r.Dependencies["search"], "choices/result") || r.Definitions["choices/result"] == "" {
		t.Fatal("choice missing from change impact")
	}
	before := r
	s.Choices["result"] = model.Choice{Label: "Changed"}
	now := Analyze(s, "test")
	if err := Compare(now, before); err != nil || !model.Contains(now.Impact.AffectedUseCases, "search") {
		t.Fatalf("choice impact: %v, %+v", err, now.Impact)
	}
	s.Project.Coverage = map[string]float64{"transitions": 100}
	if Analyze(s, "test").Status != "failed" {
		t.Fatal("missing choice branch met 100% threshold")
	}
}

func TestChoiceGuardOverlapOrGapCannotProveModel(t *testing.T) {
	for _, guard := range []bool{false, true} {
		s := testkit.ChoiceSpec()
		for _, id := range []string{"found", "empty"} {
			tr := s.Transitions[id]
			tr.Guard = guard
			s.Transitions[id] = tr
		}
		g := Explore(engine.New(s), s.Project.Initial, nil, nil)
		if g.Complete {
			t.Fatal("choice gap/overlap accepted")
		}
		found := false
		for _, d := range g.Diagnostics {
			found = found || d.Code == "event-partition"
		}
		if !found {
			t.Fatal(g.Diagnostics)
		}
	}
}

func TestChoiceIncomingPredicateDoesNotEndUseCaseBeforeBranch(t *testing.T) {
	s := testkit.ChoiceSpec()
	end := map[string]any{"var": "matched"}
	g := Explore(engine.New(s), s.Project.Initial, nil, end)
	for _, n := range g.Nodes {
		if n.State.State == "result" && n.Terminal {
			t.Fatal("choice became terminal before executing branch")
		}
	}
	s.Properties["correctAlert"] = model.Property{Kind: "invariant", Description: "Result must match alert", Assert: map[string]any{"or": []any{
		map[string]any{"eq": []any{map[string]any{"var": "$state"}, "ready"}},
		map[string]any{"eq": []any{map[string]any{"var": "alert"}, map[string]any{"not": map[string]any{"var": "matched"}}}},
	}}}
	r := Analyze(s, "test")
	if r.Status != "verified" {
		t.Fatal(r.Diagnostics, r.Checks)
	}
	tr := s.Transitions["empty"]
	tr.Set["alert"] = false
	s.Transitions["empty"] = tr
	for _, c := range Analyze(s, "test").Checks {
		if c.ID == "correctAlert" {
			if c.Status != "failed" || c.Witness == nil || len(c.Witness.Steps) != 2 {
				t.Fatal(c)
			}
			return
		}
	}
	t.Fatal("independent branch result property absent")
}

func TestChoiceIntermediateNodesStillConsumeExplorationBudget(t *testing.T) {
	s := testkit.ChoiceSpec()
	s.Project.Limits.States = 2
	r := Analyze(s, "test")
	if r.Complete || r.Status == "verified" {
		t.Fatal("transient work bypassed state limit")
	}
}
