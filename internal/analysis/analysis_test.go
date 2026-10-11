package analysis

import (
	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/testkit"
	"strings"
	"testing"
)

func TestPrimePathsLongSeparatedChoices(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument()}
	scope := map[string]bool{}
	edges := [][3]string{{"a", "s", "a"}, {"b", "s", "b"}, {"am", "a", "m"}, {"bm", "b", "m"}, {"mn", "m", "n"}, {"x", "n", "x"}, {"y", "n", "y"}, {"xe", "x", "e"}, {"ye", "y", "e"}}
	for _, ed := range edges {
		s.States[ed[1]] = model.State{}
		s.States[ed[2]] = model.State{}
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
	for _, id := range []string{"a", "b", "c"} {
		s.States[id] = model.State{}
	}
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
					// Eight transitions: the search action, its two result branches and the edit flow.
					if c.Covered != 8 || c.Total != 8 {
						t.Fatal(c)
					}
				case "primePaths":
					// Repeating the search is a prime cycle that no scenario covers.
					if c.Covered != 5 || c.Total != 6 || !hasPath(c.Missing, "searchFound", "searchCustomers") {
						t.Fatal(c)
					}
				case "boundedPaths":
					// The ten-step bound yields ten finite paths;
					// permission rejection is checked independently as a regression.
					if c.Covered != 3 || c.Total != 10 {
						t.Fatal(c)
					}
				case "pairs":
					if c.Covered != 7 || c.Total != 8 {
						t.Fatal(c)
					}
					if !hasPath(c.Missing, "searchFound", "searchCustomers") {
						t.Fatal("repeated search pair must remain uncovered")
					}
				}
			}
		}
	}
}
func hasPath(requirements []model.Requirement, path ...string) bool {
	for _, r := range requirements {
		if model.JSON(r.Path) == model.JSON(path) {
			return true
		}
	}
	return false
}
func TestDeterminism(t *testing.T) {
	s := testkit.Sample(t)
	a := model.JSON(Analyze(s, "test"))
	b := model.JSON(Analyze(s, "test"))
	if a != b {
		t.Fatal("nondeterministic report")
	}
}

// The review basis deliberately includes presentation metadata; finite results do not.
func useCasesWithoutReviewBasis(results []model.UseCaseResult) []model.UseCaseResult {
	copy := append([]model.UseCaseResult{}, results...)
	for i := range copy {
		copy[i].ReviewBasisHash = ""
	}
	return copy
}

func TestTransitionDefaultVisibleDoesNotAffectAnalysis(t *testing.T) {
	baseline := Analyze(testkit.Sample(t), "test")
	for _, visible := range []bool{false, true} {
		s := testkit.Sample(t)
		for id, transition := range s.Transitions {
			transition.DefaultVisible = visible
			s.Transitions[id] = transition
		}
		got := Analyze(s, "test")
		if baseline.Status != got.Status {
			t.Fatalf("defaultVisible=%v changed Status: %q != %q", visible, baseline.Status, got.Status)
		}
		if baseline.Complete != got.Complete {
			t.Fatalf("defaultVisible=%v changed Complete: %v != %v", visible, baseline.Complete, got.Complete)
		}
		if baseline.ReachableStates != got.ReachableStates {
			t.Fatalf("defaultVisible=%v changed ReachableStates: %d != %d", visible, baseline.ReachableStates, got.ReachableStates)
		}
		for _, fields := range []struct {
			name string
			want any
			got  any
		}{
			{name: "Diagnostics", want: baseline.Diagnostics, got: got.Diagnostics},
			{name: "Checks", want: baseline.Checks, got: got.Checks},
			{name: "Scenarios", want: baseline.Scenarios, got: got.Scenarios},
			{name: "UseCases", want: useCasesWithoutReviewBasis(baseline.UseCases), got: useCasesWithoutReviewBasis(got.UseCases)},
		} {
			if model.JSON(fields.want) != model.JSON(fields.got) {
				t.Fatalf("defaultVisible=%v changed %s", visible, fields.name)
			}
		}
	}
}

func TestStateLayoutLayerDoesNotAffectAnalysis(t *testing.T) {
	baseline := Analyze(testkit.Sample(t), "test")
	for _, layer := range []int{0, 7} {
		s := testkit.Sample(t)
		for id := range s.States {
			value := layer
			state := s.States[id]
			state.LayoutLayer = &value
			s.States[id] = state
		}
		got := Analyze(s, "test")
		if baseline.Status != got.Status {
			t.Fatalf("layoutLayer=%d changed Status: %q != %q", layer, baseline.Status, got.Status)
		}
		if baseline.Complete != got.Complete {
			t.Fatalf("layoutLayer=%d changed Complete: %v != %v", layer, baseline.Complete, got.Complete)
		}
		if baseline.ReachableStates != got.ReachableStates {
			t.Fatalf("layoutLayer=%d changed ReachableStates: %d != %d", layer, baseline.ReachableStates, got.ReachableStates)
		}
		for _, fields := range []struct {
			name string
			want any
			got  any
		}{
			{name: "Diagnostics", want: baseline.Diagnostics, got: got.Diagnostics},
			{name: "Checks", want: baseline.Checks, got: got.Checks},
			{name: "Scenarios", want: baseline.Scenarios, got: got.Scenarios},
			{name: "UseCases", want: useCasesWithoutReviewBasis(baseline.UseCases), got: useCasesWithoutReviewBasis(got.UseCases)},
		} {
			if model.JSON(fields.want) != model.JSON(fields.got) {
				t.Fatalf("layoutLayer=%d changed %s", layer, fields.name)
			}
		}
	}
}
func TestThresholdFailure(t *testing.T) {
	s := testkit.Sample(t)
	u := s.UseCases["viewCustomer"]
	delete(u.Scenarios, "disappeared")
	s.UseCases["viewCustomer"] = u
	s.Project.Coverage = map[string]float64{"primePaths": 100}
	r := Analyze(s, "test")
	if r.Status != "failed" {
		t.Fatal("uncovered prime paths passed")
	}
	for _, d := range r.Diagnostics {
		if d.Code == "coverage-threshold" && strings.Contains(d.Message, "primePaths") {
			return
		}
	}
	t.Fatal("primePaths threshold diagnostic missing")
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
	s.Transitions["duplicate"] = s.Transitions["searchFound"]
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

func convergingStartsSpec() *model.Spec {
	s := &model.Spec{Document: model.EmptyDocument(), Project: model.Project{
		SchemaVersion: "1", Name: "converging starts", Files: []string{"model.json"}, Output: "out",
		Initial: []model.Snapshot{{State: "a"}, {State: "b"}},
		Limits:  model.Limits{States: 100, Steps: 8, Paths: 10000, Combinations: 100},
	}}
	s.Screens["screen"] = model.Screen{Label: "screen"}
	s.States["a"] = model.State{Label: "a", Screen: "screen", Events: []string{"goA"}}
	s.States["b"] = model.State{Label: "b", Screen: "screen", Events: []string{"goB"}}
	s.States["middle"] = model.State{Label: "middle", Screen: "screen", Events: []string{"finish"}}
	s.States["end"] = model.State{Label: "end", Screen: "screen", Terminal: true}
	s.Events["goA"] = model.Event{Label: "goA", Source: "user"}
	s.Events["goB"] = model.Event{Label: "goB", Source: "user"}
	s.Events["finish"] = model.Event{Label: "finish", Source: "user", Parameters: map[string][]any{"confirmed": {true}}}
	s.Transitions["ta"] = model.Transition{From: "a", To: "end", Event: "goA", Action: "complete a"}
	s.Transitions["tb"] = model.Transition{From: "b", To: "middle", Event: "goB", Action: "continue b"}
	s.Transitions["finish"] = model.Transition{From: "middle", To: "end", Event: "finish", Action: "complete b"}
	end := map[string]any{"eq": []any{map[string]any{"var": "$state"}, "end"}}
	s.UseCases["flow"] = model.UseCase{
		Name: "flow", Goal: "complete", Actor: "actor", Starts: s.Project.Initial, End: end,
		Scope: []string{"ta", "tb", "finish"}, Scenarios: map[string]model.Scenario{
			"fromA": {Name: "from a", Goal: "complete from a", ExpectedOutcome: "reach end directly", Start: model.Snapshot{State: "a"}, Steps: []model.Step{{Transition: "ta"}}, Expect: end},
			"fromB": {Name: "from b", Goal: "complete from b", ExpectedOutcome: "reach end after confirmation", Start: model.Snapshot{State: "b"}, Steps: []model.Step{{Transition: "tb"}, {Transition: "finish", Input: map[string]any{"confirmed": true}}}, Expect: end},
		},
	}
	return s
}

func TestImpactIncludesOutOfScopeEventBranches(t *testing.T) {
	for _, event := range []string{"goA", "side"} {
		t.Run(event, func(t *testing.T) {
			s := convergingStartsSpec()
			if event == "side" {
				state := s.States["a"]
				state.Events = append(state.Events, event)
				s.States["a"] = state
				s.Events[event] = model.Event{Label: event, Source: "user"}
				s.Transitions["side"] = model.Transition{From: "a", To: "a", Event: event, Action: "stay", UnownedReason: "outside flow scope"}
			}
			s.Transitions["shadow"] = model.Transition{From: "a", To: "end", Event: event, Action: "alternate", Guard: false, UnownedReason: "outside flow scope"}
			old := Analyze(s, "test")
			if old.Status != "verified" {
				t.Fatalf("baseline status %s: %+v", old.Status, old.Diagnostics)
			}
			tr := s.Transitions["shadow"]
			tr.Guard = true
			s.Transitions["shadow"] = tr
			now := Analyze(s, "test")
			if now.Status != "failed" || now.UseCases[0].Complete {
				t.Fatalf("event conflict must invalidate the use-case graph: %+v", now)
			}
			if err := Compare(now, old); err != nil {
				t.Fatal(err)
			}
			if !model.Contains(now.Impact.AffectedUseCases, "flow") {
				t.Fatalf("out-of-scope event conflict omitted from affected use cases: %+v", now.Impact)
			}
		})
	}
}

func TestImpactIncludesUseCaseStartReachability(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument(), Project: model.Project{
		SchemaVersion: "1", Name: "start reachability", Files: []string{"model.json"}, Output: "out",
		Initial: []model.Snapshot{{State: "boot"}},
		Limits:  model.Limits{States: 100, Steps: 8, Paths: 10000, Combinations: 100},
	}}
	s.Screens["screen"] = model.Screen{Label: "screen"}
	for _, state := range []string{"boot", "gate", "ready"} {
		s.States[state] = model.State{Label: state, Screen: "screen", Events: []string{"go"}}
	}
	for _, state := range []string{"done", "stopped"} {
		s.States[state] = model.State{Label: state, Screen: "screen", Terminal: true}
	}
	s.Events["go"] = model.Event{Label: "go", Source: "user"}
	s.Transitions["enter"] = model.Transition{From: "boot", To: "gate", Event: "go", Action: "enter", UnownedReason: "before flow"}
	s.Transitions["prepare"] = model.Transition{From: "gate", To: "ready", Event: "go", Action: "prepare", UnownedReason: "before flow"}
	s.Transitions["finish"] = model.Transition{From: "ready", To: "done", Event: "go", Action: "finish"}
	s.UseCases["flow"] = model.UseCase{
		Name: "flow", Goal: "finish", Actor: "actor", Starts: []model.Snapshot{{State: "ready"}}, Scope: []string{"finish"},
		End: map[string]any{"eq": []any{map[string]any{"var": "$state"}, "done"}},
	}
	old := Analyze(s, "test")
	if old.Status != "verified" {
		t.Fatalf("baseline status %s: %+v", old.Status, old.Diagnostics)
	}
	tr := s.Transitions["enter"]
	tr.To = "stopped"
	s.Transitions["enter"] = tr
	now := Analyze(s, "test")
	unreachableStart := false
	for _, d := range now.Diagnostics {
		if d.Code == "unreachable-start" && d.Subject == "usecases/flow" {
			unreachableStart = true
		}
	}
	if now.Status != "failed" || !unreachableStart {
		t.Fatalf("redirecting the approach path must make the start unreachable: %+v", now)
	}
	if err := Compare(now, old); err != nil {
		t.Fatal(err)
	}
	if !model.Contains(now.Impact.AffectedUseCases, "flow") {
		t.Fatalf("unreachable use-case start omitted from affected use cases: %+v", now.Impact)
	}
}

func TestCompletionWitnessesStartAtCheckedConfiguration(t *testing.T) {
	s := convergingStartsSpec()
	r := Analyze(s, "test")
	if r.Status != "verified" {
		t.Fatalf("status %s: %+v", r.Status, r.Diagnostics)
	}
	checks := map[string]model.Check{}
	for _, c := range r.Checks {
		checks[c.ID] = c
	}
	u := s.UseCases["flow"]
	for id, scenarioID := range map[string]string{"completion/flow/1": "fromA", "completion/flow/2": "fromB"} {
		t.Run(id, func(t *testing.T) {
			c, ok := checks[id]
			if !ok || c.Status != "verified" || c.Witness == nil {
				t.Fatalf("missing verified completion witness: %+v", c)
			}
			want := u.Scenarios[scenarioID]
			if !model.Equal(c.Witness.Start, want.Start) || !model.Equal(c.Witness.Steps, want.Steps) {
				t.Fatalf("completion witness = %s; want start=%s steps=%s", model.JSON(c.Witness), model.JSON(want.Start), model.JSON(want.Steps))
			}
			sc := model.Scenario{Name: id, Goal: want.Goal, ExpectedOutcome: want.ExpectedOutcome, Start: c.Witness.Start, Steps: c.Witness.Steps, Expect: u.End}
			if replay := engine.New(s).Replay(id, sc, &u); !replay.Valid {
				t.Fatalf("completion witness cannot be replayed: %s", replay.Error)
			}
		})
	}
}
