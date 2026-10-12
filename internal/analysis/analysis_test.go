package analysis_test

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/analysis"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/testkit"
)

func coverageOf(t *testing.T, r *model.Report, usecase, kind string) model.Coverage {
	t.Helper()
	for _, u := range r.UseCases {
		if u.ID != usecase {
			continue
		}
		for _, c := range u.Coverage {
			if c.Kind == kind {
				return c
			}
		}
	}
	t.Fatalf("no %s coverage for %s", kind, usecase)
	return model.Coverage{}
}

func TestSampleIsVerified(t *testing.T) {
	r := analysis.Analyze(testkit.Sample(t), "test")
	if r.Status != "verified" || len(r.Diagnostics) != 0 {
		t.Fatalf("status %s: %+v", r.Status, r.Diagnostics)
	}
	if r.ReachableStates != 4 {
		t.Fatalf("reachable leaves: %d", r.ReachableStates)
	}
	for _, c := range r.Checks {
		if c.Status != "verified" {
			t.Errorf("check %s: %+v", c.ID, c)
		}
	}
	for _, s := range r.Scenarios {
		if !s.Valid {
			t.Errorf("scenario %s: %s", s.ID, s.Error)
		}
	}
	for usecase, total := range map[string]int{"searchCustomer": 3, "viewCustomer": 6, "editCustomer": 5, "deleteCustomer": 3, "updateNotifications": 1} {
		if c := coverageOf(t, r, usecase, "transitions"); c.Covered != total || c.Total != total {
			t.Errorf("%s transitions: %+v", usecase, c)
		}
	}
	// Repeating a search is an ordinary path that no scenario represents.
	pairs := coverageOf(t, r, "viewCustomer", "pairs")
	if pairs.Covered == pairs.Total || !strings.Contains(model.JSON(pairs.Missing), "顧客管理/顧客検索:検索\\u003e1件以上 \\u003e 顧客管理/顧客検索:検索") {
		t.Fatalf("repeated search pair should remain uncovered: %+v", pairs.Missing)
	}
}

const small = `{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},
 "states":{
  "A":{"events":{"進む":{"actors":["人"],"decision":{"branches":{"右":{"to":"B"},"左":{"to":"C"}}}}}},
  "B":{"events":{"終える":{"actors":["人"],"to":"A"}}},
  "C":{"events":{"戻る":{"actors":["人"],"to":"A"}}},
  "D":{}},
 "usecases":{"u":{"name":"U","goal":"G","actor":"人","starts":["A"],"ends":["B:終える"],"scope":["A:進む","B:終える","C:戻る"],
  "scenarios":{"right":{"name":"右","goal":"g","expectedOutcome":"o","start":"A","steps":[{"event":"進む","branches":["右"]},{"event":"終える"}]}}}}}`

func TestCoverageMissingWitnessAndReachability(t *testing.T) {
	r := analysis.Analyze(testkit.Compile(t, small), "test")
	transitions := coverageOf(t, r, "u", "transitions")
	if transitions.Total != 5 || transitions.Covered != 3 || len(transitions.Missing) != 2 {
		t.Fatalf("transitions: %+v", transitions)
	}
	missing := transitions.Missing[0]
	if missing.ID == "" || missing.Witness == nil || !model.Contains(missing.Witness.Transitions, missing.ID) {
		t.Fatalf("missing transition needs a completing witness: %+v", missing)
	}
	states := coverageOf(t, r, "u", "states")
	if states.Total != 3 || states.Covered != 2 || states.Missing[0].State != "C" {
		t.Fatalf("states: %+v", states)
	}
	found := false
	for _, d := range r.Diagnostics {
		if d.Code == "unreachable-state" && d.Subject == "states/D" {
			found = true
		}
	}
	if !found {
		t.Fatalf("unreachable state D not reported: %+v", r.Diagnostics)
	}
}

func TestCompletionFailsWithoutAnEndPath(t *testing.T) {
	r := analysis.Analyze(testkit.Compile(t, `{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},
	 "states":{"A":{"events":{"進む":{"actors":["人"],"to":"B"}}},"B":{},"C":{"events":{"終える":{"actors":["人"],"to":"A"}}}},
	 "usecases":{"u":{"name":"U","goal":"G","actor":"人","starts":["A"],"ends":["C:終える"],"scope":["A:進む","C:終える"],"scenarios":{}}}}`), "test")
	for _, c := range r.Checks {
		if c.Kind == "usecase-completion" && c.Status == "failed" && r.Status == "failed" {
			return
		}
	}
	t.Fatalf("completion should fail: %s %+v", r.Status, r.Checks)
}

func TestPathsThatCannotEndAreInfeasible(t *testing.T) {
	r := analysis.Analyze(testkit.Compile(t, `{"schemaVersion":"1","start":{"to":"A"},"actors":{"人":{}},
	 "states":{"A":{"events":{"進む":{"actors":["人"],"decision":{"branches":{"右":{"to":"B"},"左":{"to":"C"}}}}}},
	  "B":{"events":{"終える":{"actors":["人"],"to":"A"}}},"C":{}},
	 "usecases":{"u":{"name":"U","goal":"G","actor":"人","starts":["A"],"ends":["B:終える"],"scope":["A:進む","B:終える"],
	  "scenarios":{"s":{"name":"s","goal":"g","expectedOutcome":"o","start":"A","steps":[{"event":"進む","branches":["右"]},{"event":"終える"}]}}}}}`), "test")
	transitions := coverageOf(t, r, "u", "transitions")
	if len(transitions.Infeasible) != 1 || transitions.Infeasible[0].ID != "A:進む>左" || transitions.Total != 3 {
		t.Fatalf("a branch into a dead end must be listed as infeasible: %+v", transitions)
	}
}

func TestDeterministicReport(t *testing.T) {
	s := testkit.Sample(t)
	if a, b := model.JSON(analysis.Analyze(s, "test")), model.JSON(analysis.Analyze(s, "test")); a != b {
		t.Fatal("nondeterministic report")
	}
}

func TestCoverageReviewsBecomeStaleWhenTheModelChanges(t *testing.T) {
	s := testkit.Compile(t, small)
	r := analysis.Analyze(s, "test")
	transitions := coverageOf(t, r, "u", "transitions")
	u := s.UseCases["u"]
	u.CoverageReviews = map[string]model.CoverageReview{"left": {Kind: "transitions", Requirements: []string{transitions.Missing[0].ID}, Status: "addScenario", Reason: "左の結果を確認する", BasisHash: r.UseCases[0].ReviewBasisHash}}
	s.UseCases["u"] = u
	reviewed := analysis.Analyze(s, "test")
	if got := reviewed.UseCases[0].CoverageReviews; len(got) != 1 || got[0].Status != "current" {
		t.Fatalf("current review: %+v", got)
	}
	if coverageOf(t, reviewed, "u", "transitions").Covered != transitions.Covered {
		t.Fatal("a review must not change coverage")
	}
	changed := s.Transitions["C:戻る"]
	changed.Action = "やり直す"
	s.Transitions["C:戻る"] = changed
	if got := analysis.Analyze(s, "test").UseCases[0].CoverageReviews; got[0].Status != "stale" {
		t.Fatalf("review should be stale after a model change: %+v", got)
	}
}

func TestBaselineImpact(t *testing.T) {
	s := testkit.Compile(t, small)
	before := analysis.Analyze(s, "test")
	changed := s.Transitions["B:終える"]
	changed.Action = "完了を表示する"
	s.Transitions["B:終える"] = changed
	after := analysis.Analyze(s, "test")
	if err := analysis.Compare(after, before); err != nil {
		t.Fatal(err)
	}
	if !model.Contains(after.Impact.Changed, "transitions/B:終える") || !model.Contains(after.Impact.AffectedUseCases, "u") {
		t.Fatalf("impact: %+v", after.Impact)
	}
}
