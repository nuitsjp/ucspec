package analysis

import (
	"github.com/nuitsjp/usecase-spec/internal/engine"
	"github.com/nuitsjp/usecase-spec/internal/model"
	"github.com/nuitsjp/usecase-spec/internal/testkit"
	"testing"
)

func TestPrimePathsLongSeparatedChoices(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument()}
	scope := map[string]bool{}
	edges := [][3]string{{"a", "s", "a"}, {"b", "s", "b"}, {"am", "a", "m"}, {"bm", "b", "m"}, {"mn", "m", "n"}, {"x", "n", "x"}, {"y", "n", "y"}, {"xe", "x", "e"}, {"ye", "y", "e"}}
	for _, ed := range edges {
		s.Transitions[ed[0]] = model.Transition{From: ed[1], To: ed[2]}
		scope[ed[0]] = true
	}
	prime, done := PrimePaths(s, scope, 10000)
	if !done || len(prime) != 4 {
		t.Fatalf("%+v complete=%v", prime, done)
	}
	existing := [][]string{{"a", "am", "mn", "x", "xe"}, {"b", "bm", "mn", "y", "ye"}}
	for id := range scope {
		ok := false
		for _, p := range existing {
			ok = ok || containsPath(p, []string{id})
		}
		if !ok {
			t.Fatalf("edge missing %s", id)
		}
	}
	for a := range scope {
		for b := range scope {
			if s.Transitions[a].To == s.Transitions[b].From {
				ok := false
				for _, p := range existing {
					ok = ok || containsPath(p, []string{a, b})
				}
				if !ok {
					t.Fatalf("pair missing %s,%s", a, b)
				}
			}
		}
	}
	covered := 0
	for _, p := range prime {
		for _, x := range existing {
			if containsPath(x, p) {
				covered++
				break
			}
		}
	}
	if covered != 2 {
		t.Fatal(covered)
	}
}
func TestPrimeCyclesAndBudget(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument()}
	s.Transitions["ab"] = model.Transition{From: "a", To: "b"}
	s.Transitions["ba"] = model.Transition{From: "b", To: "a"}
	s.Transitions["cc"] = model.Transition{From: "c", To: "c"}
	scope := map[string]bool{"ab": true, "ba": true, "cc": true}
	p, ok := PrimePaths(s, scope, 100)
	if !ok || len(p) != 3 {
		t.Fatal(p, ok)
	}
	_, ok = PrimePaths(s, scope, 1)
	if ok {
		t.Fatal("budget ignored")
	}
}
func TestSampleCoverageDenominators(t *testing.T) {
	r := Analyze(testkit.Sample(t), "test")
	for _, u := range r.UseCases {
		if u.ID == "editCustomer" {
			for _, c := range u.Coverage {
				switch c.Kind {
				case "transitions":
					if c.Covered != 9 || c.Total != 9 {
						t.Fatal(c)
					}
				case "primePaths":
					if c.Covered != 7 || c.Total != 8 {
						t.Fatal(c)
					}
				case "boundedPaths":
					if c.Covered != 5 || c.Total != 20 {
						t.Fatal(c)
					}
				}
			}
		}
	}
}
func TestDeterminism(t *testing.T) {
	s := testkit.Sample(t)
	a := model.JSON(Analyze(s, "test"))
	b := model.JSON(Analyze(s, "test"))
	if a != b {
		t.Fatal("nondeterministic report")
	}
}
func TestThresholdFailure(t *testing.T) {
	s := testkit.Sample(t)
	s.Project.Coverage["primePaths"] = 100
	if Analyze(s, "test").Status != "failed" {
		t.Fatal("uncovered prime paths passed")
	}
}
func TestIncompleteIsNotPass(t *testing.T) {
	s := testkit.Sample(t)
	s.Project.Limits.States = 1
	r := Analyze(s, "test")
	if r.Complete || r.Status == "verified" {
		t.Fatalf("%s complete %v", r.Status, r.Complete)
	}
}
func TestImpossibleInvariantProducesWitness(t *testing.T) {
	s := testkit.Sample(t)
	s.Properties["impossible"] = model.Property{Kind: "invariant", Description: "always false", Assert: false}
	r := Analyze(s, "test")
	for _, c := range r.Checks {
		if c.ID == "impossible" {
			if c.Status != "failed" || c.Witness == nil {
				t.Fatal(c)
			}
			return
		}
	}
	t.Fatal("check absent")
}
func TestMissingEventBranch(t *testing.T) {
	s := testkit.Sample(t)
	delete(s.Transitions, "searchFound")
	g := Explore(engine.New(s), s.Project.Initial, nil, nil)
	found := false
	for _, d := range g.Diagnostics {
		if d.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Fatal("gap not found")
	}
}
func TestOverlappingGuards(t *testing.T) {
	s := testkit.Sample(t)
	var original model.Transition
	for _, tr := range s.Transitions {
		if tr.From == "search" {
			original = tr
			break
		}
	}
	s.Transitions["duplicate"] = original
	g := Explore(engine.New(s), s.Project.Initial, nil, nil)
	if len(g.Diagnostics) == 0 {
		t.Fatal("overlap not found")
	}
}
func TestImpactAndInvalidatedScenario(t *testing.T) {
	s := testkit.Sample(t)
	old := Analyze(s, "test")
	u := s.UseCases["viewCustomer"]
	id := model.Keys(u.Scenarios)[0]
	sc := u.Scenarios[id]
	sc.Expect = false
	u.Scenarios[id] = sc
	s.UseCases["viewCustomer"] = u
	now := Analyze(s, "test")
	if err := Compare(now, old); err != nil {
		t.Fatal(err)
	}
	if now.Impact == nil || len(now.Impact.InvalidatedScenarios) != 1 || !model.Contains(now.Impact.AffectedUseCases, "viewCustomer") {
		t.Fatalf("%+v", now.Impact)
	}
}
func TestInvalidScenarioCannotCover(t *testing.T) {
	s := testkit.Sample(t)
	u := s.UseCases["viewCustomer"]
	for id, sc := range u.Scenarios {
		sc.Expect = false
		u.Scenarios[id] = sc
	}
	s.UseCases["viewCustomer"] = u
	r := Analyze(s, "test")
	for _, uc := range r.UseCases {
		if uc.ID == "viewCustomer" {
			for _, c := range uc.Coverage {
				if c.Covered != 0 {
					t.Fatal(c)
				}
			}
		}
	}
}
func TestCoverageEmptyNotHundred(t *testing.T) {
	c := coverage("empty", map[string]model.Requirement{}, map[string]bool{}, true, "")
	if c.Percent != nil {
		t.Fatal("empty denominator is not 100%")
	}
}
func TestBoundedBudget(t *testing.T) {
	s := testkit.Sample(t)
	u := s.UseCases["editCustomer"]
	scope := map[string]bool{}
	for _, id := range u.Scope {
		scope[id] = true
	}
	g := Explore(engine.New(s), u.Starts, scope, u.End)
	_, ok := Bounded(g, 8, 1)
	if ok {
		t.Fatal("budget ignored")
	}
}

func TestUseCaseWithNoCompletionCannotPass(t *testing.T) {
	s := testkit.Sample(t)
	u := s.UseCases["viewCustomer"]
	u.End = false
	u.Scenarios = map[string]model.Scenario{}
	s.UseCases["viewCustomer"] = u
	r := Analyze(s, "test")
	found := false
	for _, c := range r.Checks {
		if c.Kind == "usecase-completion" && c.Status == "failed" {
			found = true
		}
	}
	if !found || r.Status != "failed" {
		t.Fatal("empty scenarios concealed nonterminating scope")
	}
}
func TestRecoverableDoesNotClaimInevitableTermination(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument()}
	s.Properties["recover"] = model.Property{Kind: "recoverable", Description: "AG EF end", Assert: map[string]any{"eq": []any{map[string]any{"var": "$state"}, "end"}}}
	g := &Graph{Complete: true, Nodes: []Node{{State: model.Snapshot{State: "loop"}}, {State: model.Snapshot{State: "end"}}}, Edges: []Edge{{From: 0, To: 0}, {From: 0, To: 1}}}
	r := Properties(engine.New(s), g)
	if len(r) != 1 || r[0].Status != "verified" {
		t.Fatal(r)
	}
}
