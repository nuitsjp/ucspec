package engine_test

import (
	"strings"
	"testing"

	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
)

func engineHierarchyExpr(operator string, value any) any {
	return map[string]any{operator: value}
}

func engineHierarchySpec() *model.Spec {
	s := &model.Spec{Document: model.EmptyDocument()}
	s.States = map[string]model.State{
		"app": {
			Label:   "App",
			Initial: "workspace",
			Events:  []string{"rootOnly", "shared"},
		},
		"workspace": {
			Label:   "Workspace",
			Parent:  "app",
			Initial: "ready",
			Events: []string{
				"parentOnly", "shared", "submitFallback", "savePriority", "conflict", "close", "reset", "rejectAncestor",
			},
		},
		"ready": {
			Label:  "Ready",
			Parent: "workspace",
			Events: []string{"leafOnly", "shared", "submitFallback", "savePriority", "conflict"},
		},
		"alternate": {
			Label:  "Alternate",
			Parent: "workspace",
			Events: []string{"leafOnly"},
		},
		"outside": {
			Label:  "Outside",
			Events: []string{"openWorkspace", "openAlternate", "openDone"},
		},
		"done": {
			Label:    "Done",
			Parent:   "workspace",
			Terminal: true,
		},
	}
	s.Events = map[string]model.Event{
		"rootOnly":       {Label: "Root only", Source: "system"},
		"parentOnly":     {Label: "Parent only", Source: "user"},
		"leafOnly":       {Label: "Leaf only", Source: "user"},
		"shared":         {Label: "Shared", Source: "user"},
		"submitFallback": {Label: "Submit", Source: "user"},
		"savePriority":   {Label: "Save", Source: "user"},
		"conflict":       {Label: "Conflict", Source: "user"},
		"close":          {Label: "Close", Source: "user"},
		"reset":          {Label: "Reset", Source: "user"},
		"rejectAncestor": {Label: "Reject", Source: "user"},
		"openWorkspace":  {Label: "Open workspace", Source: "user"},
		"openAlternate":  {Label: "Open alternate", Source: "user"},
		"openDone":       {Label: "Open done", Source: "user"},
	}
	s.Transitions = map[string]model.Transition{
		"childFallback": {
			From: "ready", To: "alternate", Event: "submitFallback", Guard: false,
		},
		"parentFallback": {
			From: "workspace", To: "outside", Event: "submitFallback", Guard: true,
		},
		"childPriority": {
			From: "ready", To: "alternate", Event: "savePriority", Guard: true,
		},
		"parentPriority": {
			From: "workspace", To: "outside", Event: "savePriority", Guard: true,
		},
		"conflictA": {
			From: "ready", To: "alternate", Event: "conflict", Guard: true,
		},
		"conflictB": {
			From: "ready", To: "outside", Event: "conflict", Guard: true,
		},
		"exitWorkspace": {
			From: "workspace", To: "outside", Event: "close",
		},
		"resetWorkspace": {
			From: "workspace", To: "workspace", Event: "reset",
		},
		"openWorkspace": {
			From: "outside", To: "workspace", Event: "openWorkspace",
		},
		"openAlternate": {
			From: "outside", To: "alternate", Event: "openAlternate",
		},
		"openDone": {
			From: "outside", To: "done", Event: "openDone",
		},
		"rejectAncestor": {
			From: "workspace", To: "outside", Event: "rejectAncestor", Guard: false,
		},
	}
	return s
}

func normalizedEngineState(t *testing.T, s *model.Spec, id string) model.Snapshot {
	t.Helper()
	st, err := s.Normalize(model.Snapshot{State: id})
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func observationTransitions(observations []model.Observation) []string {
	ids := make([]string, 0, len(observations))
	for _, observation := range observations {
		ids = append(ids, observation.Transition)
	}
	return ids
}

func TestEventsAreInheritedThroughTheWholeActiveStatePath(t *testing.T) {
	s := engineHierarchySpec()
	e := engine.New(s)
	st := normalizedEngineState(t, s, "app")

	want := []string{
		"close", "conflict", "leafOnly", "parentOnly", "rejectAncestor", "reset", "rootOnly", "savePriority", "shared", "submitFallback",
	}
	if got := e.Events(st.State); !model.Equal(got, want) {
		t.Fatalf("Events(%s) = %v; want %v", st.State, got, want)
	}
}

func TestCompositeTargetsEnterTheirDefaultLeafAndExplicitChildrenStayExplicit(t *testing.T) {
	s := engineHierarchySpec()
	e := engine.New(s)
	start := normalizedEngineState(t, s, "app")

	reset, _, err := e.Apply(start, "resetWorkspace", nil)
	if err != nil || reset.State != "ready" {
		t.Fatalf("resetWorkspace from ready = %q, %v; want ready", reset.State, err)
	}

	outside, _, err := e.Apply(start, "exitWorkspace", nil)
	if err != nil || outside.State != "outside" {
		t.Fatalf("exitWorkspace from ready = %q, %v; want outside", outside.State, err)
	}

	composite, _, err := e.Apply(outside, "openWorkspace", nil)
	if err != nil || composite.State != "ready" {
		t.Fatalf("openWorkspace = %q, %v; want the workspace initial leaf ready", composite.State, err)
	}
	explicit, _, err := e.Apply(outside, "openAlternate", nil)
	if err != nil || explicit.State != "alternate" {
		t.Fatalf("openAlternate = %q, %v; want alternate", explicit.State, err)
	}
	resetFromAlternate, _, err := e.Apply(explicit, "resetWorkspace", nil)
	if err != nil || resetFromAlternate.State != "ready" {
		t.Fatalf("resetWorkspace from alternate = %q, %v; want ready", resetFromAlternate.State, err)
	}
}

func TestTerminalLeafStopsInheritedParentEventsAndLeavesStateUnchanged(t *testing.T) {
	s := engineHierarchySpec()
	e := engine.New(s)
	outside := normalizedEngineState(t, s, "outside")

	terminal, _, err := e.Apply(outside, "openDone", nil)
	if err != nil || terminal.State != "done" {
		t.Fatalf("openDone = %q, %v; want terminal leaf done", terminal.State, err)
	}
	if got := e.Events(terminal.State); len(got) != 0 {
		t.Fatalf("Events(done) = %v; want no events for a terminal leaf", got)
	}

	next, _, err := e.Apply(terminal, "exitWorkspace", nil)
	if err == nil {
		t.Fatal("parent exit transition was accepted from a terminal leaf")
	}
	if !model.Equal(next, terminal) {
		t.Fatalf("rejected terminal transition changed state from %+v to %+v", terminal, next)
	}
}

func TestAncestorGuardRunsAfterAFalseChildGuard(t *testing.T) {
	s := engineHierarchySpec()
	e := engine.New(s)
	st := normalizedEngineState(t, s, "app")

	ids, observations, handled, err := e.Select(st, "submitFallback", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || !model.Equal(ids, []string{"parentFallback"}) {
		t.Fatalf("Select(submitFallback) = ids %v, handled %v; want parentFallback", ids, handled)
	}
	if got := observationTransitions(observations); !model.Equal(got, []string{"childFallback", "parentFallback"}) {
		t.Fatalf("guard observations = %v; want child then parent", got)
	}
	if observations[0].Guard || !observations[1].Guard {
		t.Fatalf("guard results = %+v; want false then true", observations)
	}

	next, _, err := e.Apply(st, "parentFallback", nil)
	if err != nil || next.State != "outside" {
		t.Fatalf("parentFallback = %q, %v; want outside", next.State, err)
	}
}

func TestEnabledChildSuppressesAncestorGuards(t *testing.T) {
	s := engineHierarchySpec()
	e := engine.New(s)
	st := normalizedEngineState(t, s, "app")

	ids, observations, handled, err := e.Select(st, "savePriority", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !handled || !model.Equal(ids, []string{"childPriority"}) {
		t.Fatalf("Select(savePriority) = ids %v, handled %v; want childPriority", ids, handled)
	}
	if got := observationTransitions(observations); !model.Equal(got, []string{"childPriority"}) {
		t.Fatalf("guard observations = %v; ancestor guard was evaluated", got)
	}

	next, _, err := e.Apply(st, "childPriority", nil)
	if err != nil || next.State != "alternate" {
		t.Fatalf("childPriority = %q, %v; want alternate", next.State, err)
	}
	if _, _, err := e.Apply(st, "parentPriority", nil); err == nil {
		t.Fatal("ancestor transition was accepted after an enabled child transition")
	}
}

func TestSameSourceEnabledTransitionsRemainAConflict(t *testing.T) {
	s := engineHierarchySpec()
	e := engine.New(s)
	st := normalizedEngineState(t, s, "app")

	next, observations, err := e.Apply(st, "conflictA", nil)
	if err == nil || !strings.Contains(err.Error(), "not uniquely enabled") {
		t.Fatalf("Apply(conflictA) error = %v; want a same-source conflict", err)
	}
	if !model.Equal(next, st) {
		t.Fatalf("conflicting transition changed state from %+v to %+v", st, next)
	}
	if got := observationTransitions(observations); !model.Equal(got, []string{"conflictA", "conflictB"}) {
		t.Fatalf("conflict observations = %v; want both same-source candidates", got)
	}
}

func TestReplayNormalizesParentStartAndUsesInheritedParentTransition(t *testing.T) {
	s := engineHierarchySpec()
	e := engine.New(s)
	end := engineHierarchyExpr("eq", []any{engineHierarchyExpr("var", "$state"), "outside"})
	uc := &model.UseCase{
		Starts: []model.Snapshot{{State: "workspace"}},
		Scope:  []string{"exitWorkspace"},
		End:    end,
	}
	scenario := model.Scenario{
		Name:            "close workspace",
		Goal:            "Leave the workspace",
		ExpectedOutcome: "The active workspace closes",
		Start:           model.Snapshot{State: "workspace"},
		Steps:           []model.Step{{Transition: "exitWorkspace"}},
		Expect:          end,
	}

	result := e.Replay("parent-start", scenario, uc)
	if !result.Valid {
		t.Fatalf("Replay returned invalid: %s", result.Error)
	}
	if len(result.Trace) != 2 || result.Trace[0].State != "ready" || result.Trace[1].State != "outside" {
		t.Fatalf("Replay trace = %+v; want ready then outside", result.Trace)
	}
}

func TestReplayCanRejectAnInheritedAncestorTransition(t *testing.T) {
	s := engineHierarchySpec()
	e := engine.New(s)
	ready := engineHierarchyExpr("eq", []any{engineHierarchyExpr("var", "$state"), "ready"})
	scenario := model.Scenario{
		Name:            "reject close",
		Goal:            "Keep the workspace open when closing is unavailable",
		ExpectedOutcome: "The active ready state is preserved",
		Start:           model.Snapshot{State: "workspace"},
		Steps:           []model.Step{{Transition: "rejectAncestor", Reject: true}},
		Expect:          ready,
	}

	result := e.Replay("reject-parent", scenario, nil)
	if !result.Valid {
		t.Fatalf("Replay returned invalid: %s", result.Error)
	}
	if len(result.Trace) != 2 || result.Trace[0].State != "ready" || result.Trace[1].State != "ready" {
		t.Fatalf("rejected Replay trace = %+v; want ready unchanged", result.Trace)
	}
}

func TestInStateExpressionUsesContainmentWhileStateVariableStaysLeafExact(t *testing.T) {
	s := engineHierarchySpec()
	st := normalizedEngineState(t, s, "app")
	e := engine.Evaluation{Spec: s, State: st}

	for _, test := range []struct {
		name string
		expr any
		want bool
	}{
		{name: "leaf", expr: engineHierarchyExpr("inState", "ready"), want: true},
		{name: "parent", expr: engineHierarchyExpr("inState", "workspace"), want: true},
		{name: "root", expr: engineHierarchyExpr("inState", "app"), want: true},
		{name: "outside", expr: engineHierarchyExpr("inState", "outside"), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := e.Bool(test.expr)
			if err != nil || got != test.want {
				t.Fatalf("Bool(%v) = %v, %v; want %v", test.expr, got, err, test.want)
			}
		})
	}
	leaf, err := e.Eval(engineHierarchyExpr("var", "$state"))
	if err != nil || !model.Equal(leaf, "ready") {
		t.Fatalf("$state = %v, %v; want the exact leaf ready", leaf, err)
	}
}

func TestInStateExpressionRejectsUnknownAndNonStringStates(t *testing.T) {
	s := engineHierarchySpec()
	st := normalizedEngineState(t, s, "app")
	e := engine.Evaluation{Spec: s, State: st}
	params := map[string][]any{}

	for _, test := range []struct {
		name string
		expr any
	}{
		{name: "runtime unknown", expr: engineHierarchyExpr("inState", "missing")},
		{name: "runtime non-string", expr: engineHierarchyExpr("inState", 7)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := e.Eval(test.expr); err == nil {
				t.Fatalf("Eval(%v) accepted invalid state expression", test.expr)
			}
		})
	}
	for _, test := range []struct {
		name string
		expr any
	}{
		{name: "static unknown", expr: engineHierarchyExpr("inState", "missing")},
		{name: "static non-string", expr: engineHierarchyExpr("inState", 7)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := engine.ValidateExpr(s, test.expr, params, nil); err == nil {
				t.Fatalf("ValidateExpr(%v) accepted invalid state expression", test.expr)
			}
		})
	}
}
