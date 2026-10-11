package engine

import (
	"fmt"
	"github.com/nuitsjp/ucspec/internal/model"
)

type Engine struct {
	Spec *model.Spec
	Out  map[string][]string
}

func New(s *model.Spec) *Engine {
	e := &Engine{Spec: s, Out: map[string][]string{}}
	for _, id := range model.Keys(s.Transitions) {
		t := s.Transitions[id]
		e.Out[t.From] = append(e.Out[t.From], id)
	}
	return e
}

func (e *Engine) Events(state string) []string {
	if e.Spec.IsChoice(state) {
		// The empty trigger is an automatic branch, never an external event.
		return []string{""}
	}
	if e.Spec.States[state].Terminal {
		return []string{}
	}
	events := map[string]bool{}
	for _, id := range e.Spec.StatePath(state) {
		for _, event := range e.Spec.States[id].Events {
			events[event] = true
		}
	}
	return model.Keys(events)
}
func (e *Engine) Inputs(event string) ([]map[string]any, error) {
	if event == "" {
		return []map[string]any{{}}, nil
	}
	ev, ok := e.Spec.Events[event]
	if !ok {
		return nil, fmt.Errorf("unknown event %s", event)
	}
	r := []map[string]any{{}}
	limit := e.Spec.Project.Limits.Combinations
	for _, k := range model.Keys(ev.Parameters) {
		d := ev.Parameters[k]
		if len(d) == 0 {
			return nil, fmt.Errorf("empty parameter domain %s.%s", event, k)
		}
		if len(r) > limit/len(d) {
			return nil, fmt.Errorf("parameter combinations exceed %d", limit)
		}
		n := []map[string]any{}
		for _, v := range r {
			for _, x := range d {
				q := model.CopyValues(v)
				q[k] = x
				n = append(n, q)
			}
		}
		r = n
	}
	return r, nil
}
func (e *Engine) ValidateInput(event string, in map[string]any) error {
	ev, ok := e.Spec.Events[event]
	if !ok {
		return fmt.Errorf("unknown event %s", event)
	}
	for _, k := range model.Keys(ev.Parameters) {
		d := ev.Parameters[k]
		x, ok := in[k]
		if !ok || !model.InDomain(d, x) {
			return fmt.Errorf("parameter %s.%s missing or outside domain", event, k)
		}
	}
	for _, k := range model.Keys(in) {
		if _, ok := ev.Parameters[k]; !ok {
			return fmt.Errorf("unknown parameter %s.%s", event, k)
		}
	}
	return nil
}
func (e *Engine) Select(st model.Snapshot, event string, in map[string]any) ([]string, []model.Observation, bool, error) {
	if event == "" && e.Spec.IsChoice(st.State) {
		if len(in) != 0 {
			return nil, nil, false, fmt.Errorf("choice branches do not accept event input")
		}
	} else {
		if err := e.ValidateInput(event, in); err != nil {
			return nil, nil, false, err
		}
	}
	if !model.Contains(e.Events(st.State), event) {
		return nil, nil, false, nil
	}
	ev := e.Spec.Events[event]
	eval := Evaluation{Spec: e.Spec, State: st, Input: in}
	if ev.When != nil {
		b, err := eval.Bool(ev.When)
		if err != nil {
			return nil, nil, false, err
		}
		if !b {
			return nil, nil, false, nil
		}
	}
	ids := []string{}
	obs := []model.Observation{}
	sources := e.Spec.StatePath(st.State)
	if e.Spec.IsChoice(st.State) {
		sources = []string{st.State}
	}
	for _, source := range sources {
		for _, id := range e.Out[source] {
			t := e.Spec.Transitions[id]
			if t.Event != event {
				continue
			}
			eval.Atoms = map[string]bool{}
			b := true
			var err error
			if t.Guard != nil {
				b, err = eval.Bool(t.Guard)
			}
			if err != nil {
				return nil, nil, true, fmt.Errorf("guard %s: %w", id, err)
			}
			obs = append(obs, model.Observation{Transition: id, Atoms: eval.Atoms, Guard: b})
			if b {
				ids = append(ids, id)
			}
		}
		// The most deeply nested enabled source wins. Conflicts at that
		// source are still returned for the event-partition check.
		if len(ids) > 0 {
			break
		}
	}
	return ids, obs, true, nil
}
func (e *Engine) Apply(st model.Snapshot, id string, in map[string]any) (model.Snapshot, []model.Observation, error) {
	t, ok := e.Spec.Transitions[id]
	if !ok {
		return st, nil, fmt.Errorf("unknown transition %s", id)
	}
	if !e.Spec.InState(st.State, t.From) {
		return st, nil, fmt.Errorf("%s starts at %s, not %s", id, t.From, st.State)
	}
	ids, obs, _, err := e.Select(st, t.Event, in)
	if err != nil {
		return st, obs, err
	}
	if len(ids) != 1 || ids[0] != id {
		return st, obs, fmt.Errorf("%s not uniquely enabled (enabled=%v)", id, ids)
	}
	next := model.CloneSnapshot(st)
	if t.Kind == "internal" {
		// Internal transitions keep the active configuration. In particular, a
		// transition declared on a composite state must not re-enter its initial
		// child.
		next.State = st.State
	} else {
		next.State, err = e.Spec.Enter(t.To)
		if err != nil {
			return st, obs, err
		}
	}
	eval := Evaluation{Spec: e.Spec, State: st, Input: in}
	// All assignments read the PRE-state (simultaneous assignment semantics).
	for _, k := range model.Keys(t.Set) {
		v, err := eval.Eval(t.Set[k])
		if err != nil {
			return st, obs, fmt.Errorf("effect %s.%s: %w", id, k, err)
		}
		d, ok := e.Spec.Variables[k]
		if !ok || !model.InDomain(d.Domain, v) {
			return st, obs, fmt.Errorf("effect %s.%s leaves finite domain", id, k)
		}
		next.Values[k] = v
	}
	return next, obs, nil
}
func (e *Engine) Replay(id string, sc model.Scenario, uc *model.UseCase) model.ScenarioResult {
	r := model.ScenarioResult{ID: id, Name: sc.Name, Goal: sc.Goal, ExpectedOutcome: sc.ExpectedOutcome, Expect: sc.Expect, Witness: model.Witness{Start: sc.Start, Steps: sc.Steps}, Trace: []model.Snapshot{}}
	st, err := e.Spec.Normalize(sc.Start)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	if e.Spec.IsChoice(st.State) {
		r.Error = "scenario must start in a state, not a choice"
		return r
	}
	r.Witness.Start = st
	r.Trace = append(r.Trace, st)
	if uc != nil {
		matched := false
		for _, x := range uc.Starts {
			n, er := e.Spec.Normalize(x)
			if er == nil && model.Equal(n, st) {
				matched = true
			}
		}
		if !matched {
			r.Error = "scenario start is not a declared use-case start"
			return r
		}
	}
	for i, step := range sc.Steps {
		if uc != nil {
			if !e.Spec.IsChoice(st.State) {
				b, er := Predicate(e.Spec, st, uc.End)
				if er != nil {
					r.Error = er.Error()
					return r
				}
				if b {
					r.Error = fmt.Sprintf("step %d continues past use-case end", i+1)
					return r
				}
			}
			if !model.Contains(uc.Scope, step.Transition) {
				r.Error = "transition outside use-case scope: " + step.Transition
				return r
			}
			if step.Reject {
				r.Error = "reject steps belong in regressions, not coverage scenarios"
				return r
			}
		}
		if step.Reject {
			if e.Spec.IsChoice(st.State) {
				r.Error = "choice must resolve before another event or rejection assertion"
				return r
			}
			t, ok := e.Spec.Transitions[step.Transition]
			if !ok || !e.Spec.InState(st.State, t.From) {
				r.Error = "rejection must target a transition from current state"
				return r
			}
			ids, _, _, er := e.Select(st, t.Event, step.Input)
			if er != nil {
				r.Error = er.Error()
				return r
			}
			if model.Contains(ids, step.Transition) {
				r.Error = "expected rejected transition is enabled: " + step.Transition
				return r
			}
		} else {
			next, obs, er := e.Apply(st, step.Transition, step.Input)
			if er != nil {
				r.Error = fmt.Sprintf("step %d: %v", i+1, er)
				return r
			}
			st = next
			r.GuardObservations = append(r.GuardObservations, obs...)
		}
		r.Trace = append(r.Trace, st)
		if step.Expect != nil {
			ok, er := Predicate(e.Spec, st, step.Expect)
			if er != nil || !ok {
				r.Error = fmt.Sprintf("step %d expectation failed: %v", i+1, er)
				return r
			}
		}
	}
	if e.Spec.IsChoice(st.State) {
		r.Error = "scenario ends at an unresolved choice"
		return r
	}
	if uc != nil {
		b, er := Predicate(e.Spec, st, uc.End)
		if er != nil || !b {
			r.Error = "scenario did not reach a use-case end"
			return r
		}
	}
	b, err := Predicate(e.Spec, st, sc.Expect)
	if err != nil || !b {
		r.Error = fmt.Sprintf("final expectation failed: %v", err)
		return r
	}
	r.Valid = true
	return r
}
