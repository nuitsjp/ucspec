package analysis

import (
	"fmt"
	"github.com/nuitsjp/ucspec/internal/engine"
	"github.com/nuitsjp/ucspec/internal/model"
)

type Node struct {
	State    model.Snapshot
	Witness  model.Witness
	Terminal bool
}
type Edge struct {
	From int
	To   int
	Step model.Step
}
type Graph struct {
	Nodes        []Node
	Edges        []Edge
	Out          map[int][]int
	Starts       []int
	Observations []model.Observation
	Complete     bool
	Diagnostics  []model.Diagnostic
}

// transitionTargets returns the stable control states reached by a transition.
// External transitions enter a composite target through its initial child;
// internal transitions preserve each currently active leaf of their source.
func transitionTargets(s *model.Spec, t model.Transition) []string {
	if t.Kind == "internal" {
		return s.Leaves(t.From)
	}
	to, _ := s.Enter(t.To)
	return []string{to}
}

func Explore(e *engine.Engine, starts []model.Snapshot, scope map[string]bool, end model.Expr) *Graph {
	g := &Graph{Out: map[int][]int{}, Complete: true}
	ids := map[string]int{}
	limit := e.Spec.Project.Limits.States
	add := func(st model.Snapshot, w model.Witness) (int, bool) {
		key := model.JSON(st)
		if id, ok := ids[key]; ok {
			return id, false
		}
		if len(g.Nodes) >= limit {
			g.Complete = false
			return -1, false
		}
		id := len(g.Nodes)
		term := e.Spec.States[st.State].Terminal
		if end != nil && !e.Spec.IsChoice(st.State) {
			b, err := engine.Predicate(e.Spec, st, end)
			if err != nil {
				g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "end-expression", Severity: "error", Message: err.Error(), Witness: &w})
				g.Complete = false
			} else {
				term = b
			}
		}
		ids[key] = id
		g.Nodes = append(g.Nodes, Node{State: st, Witness: w, Terminal: term})
		return id, true
	}
	for _, st := range starts {
		n, err := e.Spec.Normalize(st)
		if err != nil {
			g.Complete = false
			g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "initial", Severity: "error", Message: err.Error()})
			continue
		}
		if e.Spec.IsChoice(n.State) {
			g.Complete = false
			g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "initial", Severity: "error", Message: "exploration must start in a state, not a choice"})
			continue
		}
		id, _ := add(n, model.Witness{Start: n, Steps: []model.Step{}})
		if id >= 0 && !containsInt(g.Starts, id) {
			g.Starts = append(g.Starts, id)
		}
	}
	work := 0
exploration:
	for i := 0; i < len(g.Nodes); i++ {
		n := g.Nodes[i]
		if n.Terminal {
			continue
		}
		for _, evID := range e.Events(n.State.State) {
			inputs, err := e.Inputs(evID)
			if err != nil {
				g.Complete = false
				g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "input-limit", Severity: "error", Subject: "events/" + evID, Message: err.Error(), Witness: copyWitness(n.Witness)})
				continue
			}
			for _, in := range inputs {
				work++
				if work > e.Spec.Project.Limits.Paths {
					g.Complete = false
					break exploration
				}
				enabled, obs, available, err := e.Select(n.State, evID, in)
				if err != nil {
					g.Complete = false
					g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "guard-evaluation", Severity: "error", Subject: "events/" + evID, Message: err.Error(), Witness: copyWitness(n.Witness)})
					continue
				}
				// Scope restricts paths, never the interpretation of event determinism.
				for _, o := range obs {
					if scope == nil || scope[o.Transition] {
						g.Observations = append(g.Observations, o)
					}
				}
				if !available {
					continue
				}
				if len(enabled) != 1 {
					g.Complete = false // an invalid event partition cannot prove other paths infeasible
					g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "event-partition", Severity: "error", Subject: "states/" + n.State.State, Message: fmt.Sprintf("event %s input %s enables %d transitions: %v", evID, model.JSON(in), len(enabled), enabled), Witness: copyWitness(n.Witness)})
					continue
				}
				tr := enabled[0]
				if scope != nil && !scope[tr] {
					continue
				}
				next, _, err := e.Apply(n.State, tr, in)
				if err != nil {
					g.Complete = false
					g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "effect", Severity: "error", Subject: "transitions/" + tr, Message: err.Error(), Witness: copyWitness(n.Witness)})
					continue
				}
				step := model.Step{Transition: tr, Input: in}
				w := extend(n.Witness, step)
				to, _ := add(next, w)
				if to < 0 {
					continue
				}
				j := len(g.Edges)
				g.Edges = append(g.Edges, Edge{From: i, To: to, Step: step})
				g.Out[i] = append(g.Out[i], j)
			}
		}
	}
	if !g.Complete {
		g.Diagnostics = append(g.Diagnostics, model.Diagnostic{Code: "exploration-incomplete", Severity: "error", Message: fmt.Sprintf("state exploration incomplete (%d states, limit %d); not a proof", len(g.Nodes), limit)})
	}
	return g
}
func copyWitness(w model.Witness) *model.Witness {
	r := w
	r.Steps = append([]model.Step{}, w.Steps...)
	return &r
}
func extend(w model.Witness, s model.Step) model.Witness {
	return model.Witness{Start: w.Start, Steps: append(append([]model.Step{}, w.Steps...), s)}
}
func containsInt(s []int, n int) bool {
	for _, v := range s {
		if v == n {
			return true
		}
	}
	return false
}
func sortedStrings(s []string) []string {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return model.Keys(m)
}

// WitnessFor uses a product of the finite execution graph and a sequence matcher.
// It finds a COMPLETE scenario containing path, without imposing a depth bound.
// feasible=false is only conclusive when complete=true.
func WitnessFor(g *Graph, path []string, budget int) (w *model.Witness, feasible, complete bool) {
	type product struct {
		Node     int
		Progress int
		W        model.Witness
	}
	queue := []product{}
	seen := map[[2]int]bool{}
	for _, n := range g.Starts {
		k := [2]int{n, 0}
		seen[k] = true
		queue = append(queue, product{Node: n, W: model.Witness{Start: g.Nodes[n].State, Steps: []model.Step{}}})
	}
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		if p.Progress == len(path) && g.Nodes[p.Node].Terminal {
			return copyWitness(p.W), true, true
		}
		if g.Nodes[p.Node].Terminal {
			continue
		}
		for _, ei := range g.Out[p.Node] {
			ed := g.Edges[ei]
			q := advance(path, p.Progress, ed.Step.Transition)
			k := [2]int{ed.To, q}
			if seen[k] {
				continue
			}
			if len(seen) >= budget {
				return nil, false, false
			}
			seen[k] = true
			queue = append(queue, product{ed.To, q, extend(p.W, ed.Step)})
		}
	}
	return nil, false, g.Complete
}
func advance(pattern []string, q int, tr string) int {
	if q == len(pattern) {
		return q
	}
	prefix := append(append([]string{}, pattern[:q]...), tr)
	for n := min(len(pattern), len(prefix)); n > 0; n-- {
		ok := true
		for i := 0; i < n; i++ {
			if prefix[len(prefix)-n+i] != pattern[i] {
				ok = false
				break
			}
		}
		if ok {
			return n
		}
	}
	return 0
}
