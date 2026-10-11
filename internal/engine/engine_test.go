package engine_test

import (
	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
	"github.com/nuitsjp/ucspec/internal/testkit"
	"testing"
)

func op(k string, v any) any { return map[string]any{k: v} }
func TestExpressionOperators(t *testing.T) {
	s := testkit.Sample(t)
	st, _ := s.Normalize(model.Snapshot{State: "search"})
	e := engine.Evaluation{Spec: s, State: st, Input: map[string]any{"x": 2.0}}
	cases := []struct {
		name string
		x    any
		want any
	}{
		{"equal", op("eq", []any{2.0, 2}), true}, {"not", op("not", false), true}, {"and", op("and", []any{true, false}), false},
		{"or", op("or", []any{false, true}), true}, {"lt", op("lt", []any{1, 2}), true}, {"le", op("le", []any{2, 2}), true},
		{"gt", op("gt", []any{3, 2}), true}, {"ge", op("ge", []any{2, 2}), true}, {"ne", op("ne", []any{"a", "b"}), true},
		{"param", op("param", "x"), 2.0}, {"state", op("var", "$state"), "search"}}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := e.Eval(c.x)
			if err != nil || !model.Equal(v, c.want) {
				t.Fatalf("got %v %v", v, err)
			}
		})
	}
}
func TestExpressionRejectsInvalidData(t *testing.T) {
	s := testkit.Sample(t)
	e := engine.Evaluation{Spec: s, State: model.Snapshot{State: "search", Values: map[string]any{}}}
	for _, x := range []any{nil, op("exec", "rm"), op("var", "missing"), op("param", "missing"), op("and", []any{}), op("eq", []any{1}), op("not", "text"), op("lt", []any{"x", 1})} {
		if _, err := e.Eval(x); err == nil {
			t.Errorf("accepted %#v", x)
		}
	}
	s.Conditions["cycle"] = op("condition", "cycle")
	if _, err := e.Eval(op("condition", "cycle")); err == nil {
		t.Fatal("condition cycle accepted")
	}
}
func TestLogicalCoverageDoesNotShortCircuit(t *testing.T) {
	s := testkit.Sample(t)
	s.Conditions["a"] = false
	s.Conditions["b"] = true
	e := engine.Evaluation{Spec: s, Atoms: map[string]bool{}}
	v, err := e.Bool(op("and", []any{op("condition", "a"), op("condition", "b")}))
	if err != nil || v || len(e.Atoms) != 2 || !e.Atoms["b"] {
		t.Fatalf("%v %v %+v", v, err, e.Atoms)
	}
}
func TestAssignmentsAreSimultaneous(t *testing.T) {
	s := &model.Spec{Document: model.EmptyDocument(), Project: model.Project{Limits: model.Limits{Combinations: 8}}}
	s.Variables["a"] = model.Variable{Domain: []any{0, 1}, Initial: 0}
	s.Variables["b"] = model.Variable{Domain: []any{0, 1}, Initial: 1}
	s.States["x"] = model.State{Events: []string{"go"}}
	s.Events["go"] = model.Event{}
	s.Transitions["swap"] = model.Transition{From: "x", To: "x", Event: "go", Set: map[string]any{"a": op("var", "b"), "b": op("var", "a")}}
	st, _ := s.Normalize(model.Snapshot{State: "x"})
	next, _, err := engine.New(s).Apply(st, "swap", nil)
	if err != nil || !model.Equal(next.Values["a"], 1) || !model.Equal(next.Values["b"], 0) || !model.Equal(st.Values["a"], 0) {
		t.Fatalf("%+v %v", next, err)
	}
	tr := s.Transitions["swap"]
	tr.Set["a"] = 2
	s.Transitions["swap"] = tr
	if _, _, err = engine.New(s).Apply(st, "swap", nil); err == nil {
		t.Fatal("out-of-domain update accepted")
	}
}
func TestInputDomainAndBudget(t *testing.T) {
	s := testkit.Sample(t)
	s.Events["test"] = model.Event{Parameters: map[string][]any{"a": {false, true}, "b": {0, 1}}}
	e := engine.New(s)
	in, err := e.Inputs("test")
	if err != nil || len(in) != 4 {
		t.Fatal(in, err)
	}
	for _, bad := range []map[string]any{{"a": true}, {"a": true, "b": 2}, {"a": true, "b": 0, "c": true}} {
		if e.ValidateInput("test", bad) == nil {
			t.Fatal("invalid input accepted")
		}
	}
	s.Project.Limits.Combinations = 2
	if _, err = e.Inputs("test"); err == nil {
		t.Fatal("budget ignored")
	}
}
func TestReplayRejectsWrongStartScopeAndExpectation(t *testing.T) {
	s := testkit.Sample(t)
	u := s.UseCases["viewCustomer"]
	id := model.Keys(u.Scenarios)[0]
	base := u.Scenarios[id]
	e := engine.New(s)
	if r := e.Replay("ok", base, &u); !r.Valid {
		t.Fatal(r.Error)
	}
	for _, mode := range []string{"start", "scope", "expect", "reject"} {
		t.Run(mode, func(t *testing.T) {
			sc := base
			sc.Steps = append([]model.Step{}, base.Steps...)
			uc := u
			switch mode {
			case "start":
				sc.Start.State = "unknown"
			case "scope":
				uc.Scope = []string{}
			case "expect":
				sc.Expect = false
			case "reject":
				sc.Steps[0].Reject = true
			}
			if r := e.Replay(mode, sc, &uc); r.Valid {
				t.Fatal("invalid scenario counted")
			}
		})
	}
}
