// Package engine moves through the compiled state structure. It knows no data:
// a configuration is an active leaf, or a decision being resolved.
package engine

import (
	"fmt"
	"strings"

	"github.com/nuitsjp/ucspec/internal/model"
)

type Engine struct {
	Spec *model.Spec
}

func New(s *model.Spec) *Engine { return &Engine{Spec: s} }

// Edge is one transition from a configuration and the configuration it leads to.
type Edge struct {
	Transition string
	Next       model.Snapshot
}

// Available returns the events a leaf accepts by name. Events of ancestors are
// inherited; an event declared closer to the leaf overrides one with the same name.
func (e *Engine) Available(leaf string) map[string]string {
	events := map[string]string{}
	for _, path := range e.Spec.StatePath(leaf) {
		for name, id := range e.Spec.States[path].Events {
			if _, ok := events[name]; !ok {
				events[name] = id
			}
		}
	}
	return events
}

// Outgoing lists the transitions leaving a configuration. From a leaf only the
// events the actor may raise are returned; an empty actor allows every event.
func (e *Engine) Outgoing(v model.Snapshot, actor string) ([]Edge, error) {
	edges := []Edge{}
	if d, ok := e.Spec.Decisions[v.State]; ok {
		for _, id := range d.Branches {
			next, err := e.next(id, v.Origin)
			if err != nil {
				return nil, err
			}
			edges = append(edges, Edge{id, next})
		}
		return edges, nil
	}
	available := e.Available(v.State)
	for _, name := range model.Keys(available) {
		id := available[name]
		if actor != "" && !model.Contains(e.Spec.Transitions[id].Actors, actor) {
			continue
		}
		next, err := e.next(id, v.State)
		if err != nil {
			return nil, err
		}
		edges = append(edges, Edge{id, next})
	}
	return edges, nil
}

// next resolves where a transition leads. Without a destination the
// configuration returns to the leaf that raised the event.
func (e *Engine) next(id, origin string) (model.Snapshot, error) {
	t := e.Spec.Transitions[id]
	switch {
	case t.To == "":
		if origin == "" {
			return model.Snapshot{}, fmt.Errorf("%s has no destination", id)
		}
		return model.Snapshot{State: origin}, nil
	case e.Spec.IsDecision(t.To):
		return model.Snapshot{State: t.To, Origin: origin}, nil
	}
	entered, err := e.Spec.Enter(t.To)
	if err != nil {
		return model.Snapshot{}, err
	}
	if e.Spec.IsDecision(entered) {
		return model.Snapshot{State: entered, Origin: origin}, nil
	}
	return model.Snapshot{State: entered}, nil
}

// Start enters a state for a scenario or exploration start.
func (e *Engine) Start(state string) (model.Snapshot, error) {
	entered, err := e.Spec.Enter(state)
	if err != nil {
		return model.Snapshot{}, err
	}
	return model.Snapshot{State: entered}, nil
}

// Play performs an authored step: the event, then the named branch at each
// decision passed on the way, in order.
func (e *Engine) Play(v model.Snapshot, step model.Step, actor string) (model.Snapshot, []string, error) {
	if e.Spec.IsDecision(v.State) {
		return v, nil, fmt.Errorf("a decision is unresolved before event %q", step.Event)
	}
	id, ok := e.Available(v.State)[step.Event]
	allowed := ok && (actor == "" || model.Contains(e.Spec.Transitions[id].Actors, actor))
	if step.Reject {
		if allowed {
			return v, nil, fmt.Errorf("event %q was expected to be rejected in %s", step.Event, v.State)
		}
		return v, nil, nil
	}
	if !ok {
		return v, nil, fmt.Errorf("event %q is not available in %s", step.Event, v.State)
	}
	if !allowed {
		return v, nil, fmt.Errorf("%s cannot raise event %q", actor, step.Event)
	}
	path := []string{id}
	current, err := e.next(id, v.State)
	if err != nil {
		return v, nil, err
	}
	used := 0
	for e.Spec.IsDecision(current.State) {
		d := e.Spec.Decisions[current.State]
		names := []string{}
		for _, b := range d.Branches {
			names = append(names, e.Spec.Transitions[b].Name)
		}
		if used >= len(step.Branches) {
			return v, nil, fmt.Errorf("decision %q needs a branch: one of %s", e.Spec.Label(current.State), strings.Join(names, ", "))
		}
		branch := model.BranchID(d.Owner, step.Branches[used])
		if !model.Contains(d.Branches, branch) {
			return v, nil, fmt.Errorf("decision %q has no branch %q (one of %s)", e.Spec.Label(current.State), step.Branches[used], strings.Join(names, ", "))
		}
		used++
		path = append(path, branch)
		if current, err = e.next(branch, current.Origin); err != nil {
			return v, nil, err
		}
	}
	if used < len(step.Branches) {
		return v, nil, fmt.Errorf("event %q passes no decision for branch %q", step.Event, step.Branches[used])
	}
	return current, path, nil
}

// Replay plays a scenario. A use-case scenario must start at a declared start,
// stay within the scope and stop at the step that takes an end transition.
func (e *Engine) Replay(id string, sc model.Scenario, uc *model.UseCase) model.ScenarioResult {
	actor := sc.Actor
	if uc != nil {
		actor = uc.Actor
	}
	r := model.ScenarioResult{ID: id, Name: sc.Name, Goal: sc.Goal, ExpectedOutcome: sc.ExpectedOutcome, Actor: actor, Steps: sc.Steps, Trace: []model.Snapshot{}, Witness: model.Witness{Transitions: []string{}}}
	fail := func(format string, args ...any) model.ScenarioResult {
		r.Error = fmt.Sprintf(format, args...)
		return r
	}
	if uc != nil && !model.Contains(uc.Starts, sc.Start) {
		return fail("scenario start is not a declared use-case start")
	}
	v, err := e.Start(sc.Start)
	if err != nil {
		return fail("%v", err)
	}
	if e.Spec.IsDecision(v.State) {
		return fail("scenario start must enter a state without a decision")
	}
	r.Witness.Start = v.State
	r.Trace = append(r.Trace, v)
	ended := false
	for i, step := range sc.Steps {
		if ended {
			return fail("step %d continues past use-case end", i+1)
		}
		if uc != nil && step.Reject {
			return fail("reject steps belong in regressions, not use-case scenarios")
		}
		next, path, err := e.Play(v, step, actor)
		if err != nil {
			return fail("step %d: %v", i+1, err)
		}
		for _, t := range path {
			if uc != nil && e.Spec.Transitions[t].Kind == model.KindEvent && !model.Contains(uc.Scope, t) {
				return fail("step %d: event %s is outside the use-case scope", i+1, t)
			}
			if uc != nil && model.Contains(uc.Ends, t) {
				ended = true
			}
		}
		r.Witness.Transitions = append(r.Witness.Transitions, path...)
		v = next
		r.Trace = append(r.Trace, v)
	}
	if uc != nil && !ended {
		return fail("scenario did not reach a use-case end")
	}
	r.Valid = true
	return r
}
